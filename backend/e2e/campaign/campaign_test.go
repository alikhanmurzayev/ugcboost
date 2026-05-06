// Package campaign — E2E тесты HTTP-поверхности /campaigns.
//
// TestCreateCampaign проходит POST /campaigns во всех задокументированных
// ответах. Без токена ручка возвращает 401 — публичный доступ к админ-каталогу
// кампаний закрыт middleware'ом ещё до handler'а. От лица brand_manager —
// 403 FORBIDDEN: создавать кампании в текущем MVP может только админ
// (brand-self-service-флоу выпал из роудмапа). Затем сетка валидаций для
// admin-токена: пустое имя (после trim) уходит сырым HTTP, чтобы дойти до
// серверной валидации, и возвращает 422 CAMPAIGN_NAME_REQUIRED; имя длиной
// >255 рун — 422 CAMPAIGN_NAME_TOO_LONG; пустой tmaUrl — 422
// CAMPAIGN_TMA_URL_REQUIRED; tmaUrl длиннее 2048 — 422
// CAMPAIGN_TMA_URL_TOO_LONG. Каждый код актуален для подсказки на форме.
// Happy-path: 201 + полный Campaign с server-stamped uuid, isDeleted=false и
// двумя совпадающими timestamp'ами, плюс audit-row campaign_create в той же
// транзакции (проверка через testutil.AssertAuditEntry).
//
// TestCreateCampaign_RaceUniqueName закрывает партиальный UNIQUE индекс
// campaigns_name_active_unique (WHERE is_deleted = false). Два concurrent
// POST'а с одинаковым name запускаются в горутинах: ровно один получает 201,
// другой — 409 CAMPAIGN_NAME_TAKEN с actionable RU-message. Без этого теста
// EAFP-обработка 23505 в repo осталась бы незакрытой согласно
// backend-testing-e2e.md § Race-сценарии.
//
// Сетап компонуется через testutil.SetupAdminClient + SetupCampaign и
// SetupBrand + SetupManagerWithLogin для 403-кейса; созданные кампании
// автоматически снимаются после теста через POST /test/cleanup-entity при
// E2E_CLEANUP=true (дефолт). Имена кампаний уникализируются через
// testutil.UniqueEmail чтобы тест проходил на любом состоянии БД.
package campaign

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

const validTmaURL = "https://tma.ugcboost.kz/tz/abc123secret"

func TestCreateCampaign(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		// Raw HTTP: the generated client's WithAuth options force a Bearer
		// header; we deliberately send no Authorization to exercise the
		// middleware short-circuit.
		resp := testutil.PostRaw(t, "/campaigns", apiclient.CampaignInput{
			Name:   "Promo-" + testutil.UniqueEmail("unauth"),
			TmaUrl: validTmaURL,
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "HostBrand-"+testutil.UniqueEmail("host"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		resp, err := mgrClient.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   "Promo-" + testutil.UniqueEmail("mgrtries"),
			TmaUrl: validTmaURL,
		}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("empty name returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   "   ",
			TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_NAME_REQUIRED", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "Название кампании обязательно")
	})

	t.Run("name too long returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   strings.Repeat("A", 256),
			TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_NAME_TOO_LONG", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "слишком длинное")
	})

	t.Run("empty tmaUrl returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   "Promo-" + testutil.UniqueEmail("emptyurl"),
			TmaUrl: "   ",
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_TMA_URL_REQUIRED", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "Ссылка на TMA-страницу обязательна")
	})

	t.Run("tmaUrl too long returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   "Promo-" + testutil.UniqueEmail("longurl"),
			TmaUrl: strings.Repeat("A", 2049),
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_TMA_URL_TOO_LONG", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "слишком длинная")
	})

	t.Run("success returns 201 with full payload and writes audit row", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		name := "Promo-" + testutil.UniqueEmail("happy")

		resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   name,
			TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.NotNil(t, resp.JSON201)

		got := resp.JSON201.Data
		require.NotEqual(t, uuid.Nil, got.Id, "server-stamped uuid must be present")
		require.Equal(t, name, got.Name)
		require.Equal(t, validTmaURL, got.TmaUrl)
		require.False(t, got.IsDeleted)
		now := time.Now().UTC()
		const recentWindow = 5 * time.Minute
		require.WithinDuration(t, now, got.CreatedAt, recentWindow)
		require.WithinDuration(t, now, got.UpdatedAt, recentWindow)
		// Same-tx insert means the two timestamps are identical at row birth.
		require.Equal(t, got.CreatedAt, got.UpdatedAt)

		expected := apiclient.Campaign{
			Id:        got.Id,
			Name:      name,
			TmaUrl:    validTmaURL,
			IsDeleted: false,
			CreatedAt: got.CreatedAt,
			UpdatedAt: got.UpdatedAt,
		}
		require.Equal(t, expected, got)

		testutil.RegisterCampaignCleanup(t, got.Id.String())
		testutil.AssertAuditEntry(t, c, token, "campaign", got.Id.String(), "campaign_create")
	})
}

func TestCreateCampaign_RaceUniqueName(t *testing.T) {
	t.Parallel()
	c, token, _ := testutil.SetupAdminClient(t)
	name := "Promo-Race-" + testutil.UniqueEmail("race")

	var (
		wg          sync.WaitGroup
		ok201       atomic.Int32
		ok409       atomic.Int32
		winnerLock  sync.Mutex
		winnerUUID  uuid.UUID
		otherStatus atomic.Int32
	)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
				Name:   name,
				TmaUrl: validTmaURL,
			}, testutil.WithAuth(token))
			require.NoError(t, err)
			switch resp.StatusCode() {
			case http.StatusCreated:
				ok201.Add(1)
				require.NotNil(t, resp.JSON201)
				winnerLock.Lock()
				winnerUUID = resp.JSON201.Data.Id
				winnerLock.Unlock()
			case http.StatusConflict:
				require.NotNil(t, resp.JSON409)
				require.Equal(t, "CAMPAIGN_NAME_TAKEN", resp.JSON409.Error.Code)
				require.Contains(t, resp.JSON409.Error.Message, "Кампания с таким названием")
				ok409.Add(1)
			default:
				otherStatus.Store(int32(resp.StatusCode()))
			}
		}()
	}
	wg.Wait()

	require.Zero(t, otherStatus.Load(), "no 5xx / unexpected status under race")
	require.Equal(t, int32(1), ok201.Load(), "exactly one create must succeed")
	require.Equal(t, int32(1), ok409.Load(), "exactly one create must lose the race with CAMPAIGN_NAME_TAKEN")
	require.NotEqual(t, uuid.Nil, winnerUUID)
	testutil.RegisterCampaignCleanup(t, winnerUUID.String())
}
