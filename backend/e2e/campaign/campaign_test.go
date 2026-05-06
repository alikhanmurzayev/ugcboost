// Package campaign — E2E тесты HTTP-поверхности /campaigns.
//
// TestCampaignCRUD проходит все три ручки кампаний (POST /campaigns,
// GET /campaigns/{id}, PATCH /campaigns/{id}) во всех задокументированных
// ответах. Без токена POST возвращает 401 — публичный доступ к админ-каталогу
// кампаний закрыт middleware'ом ещё до handler'а. От лица brand_manager — 403
// FORBIDDEN: создавать кампании в текущем MVP может только админ
// (brand-self-service-флоу выпал из роудмапа). Затем сетка валидаций для
// admin-токена: пустое имя (после trim) уходит сырым HTTP, чтобы дойти до
// серверной валидации, и возвращает 422 CAMPAIGN_NAME_REQUIRED; имя длиной
// >255 рун — 422 CAMPAIGN_NAME_TOO_LONG; пустой tmaUrl — 422
// CAMPAIGN_TMA_URL_REQUIRED; tmaUrl длиннее 2048 — 422
// CAMPAIGN_TMA_URL_TOO_LONG. Каждый код актуален для подсказки на форме.
// Happy-path POST: 201 + id-only payload с server-stamped uuid плюс audit-row
// campaign_create в той же транзакции (проверка через testutil.FindAuditEntry).
//
// GET /campaigns/{id} — admin-only read-by-id. Без токена middleware
// возвращает 401 ещё до handler'а; brand_manager-токен ловит 403 FORBIDDEN
// от authz-сервиса (timing-safe, до DB-чтения), несуществующий uuid — 404
// CAMPAIGN_NOT_FOUND с RU-сообщением «Кампания не найдена.». Happy-path:
// после POST из соседнего t.Run (или inline) GET по тому же id отдаёт 200 +
// полный Campaign — id совпадает с создающим ответом, name/tmaUrl равны
// исходным значениям, isDeleted=false для свежесозданной кампании, оба
// timestamp'а близки к моменту POST (через WithinDuration ~1 мин).
//
// PATCH /campaigns/{id} — admin-only full-replace мутируемого подмножества
// (`name`, `tmaUrl`). Без токена middleware возвращает 401, brand_manager —
// 403 ещё до DB-чтения, несуществующий uuid — 404 CAMPAIGN_NOT_FOUND с
// тем же RU-сообщением. Сетка валидаций повторяет POST: пустое имя / >255 /
// пустой tmaUrl / >2048 — каждый со своим granular кодом
// (CAMPAIGN_NAME_REQUIRED / TOO_LONG / TMA_URL_REQUIRED / TOO_LONG). Имя,
// уже занятое другой live-кампанией, возвращает 409 CAMPAIGN_NAME_TAKEN —
// обработка race на partial unique уже покрыта TestCreateCampaign_RaceUniqueName,
// поэтому отдельный race-тест на UPDATE не дублируется. Happy-path: PATCH
// отвечает 204 без тела, последующий GET по тому же id показывает
// обновлённые name/tmaUrl и updatedAt > createdAt; в audit_logs появляется
// отдельная строка campaign_update со старыми и новыми значениями в
// old_value/new_value (round-trip JSON для сравнения).
//
// TestCreateCampaign_RaceUniqueName закрывает партиальный UNIQUE индекс
// campaigns_name_active_unique (WHERE is_deleted = false). Два concurrent
// POST'а с одинаковым name запускаются в горутинах: ровно один получает 201,
// другой — 409 CAMPAIGN_NAME_TAKEN с actionable RU-message. Без этого теста
// EAFP-обработка 23505 в repo осталась бы незакрытой согласно
// backend-testing-e2e.md § Race-сценарии.
//
// Сетап компонуется через testutil.SetupAdminClient + SetupBrand +
// SetupManagerWithLogin для 403-кейсов; созданные кампании автоматически
// снимаются после теста через POST /test/cleanup-entity при E2E_CLEANUP=true
// (дефолт). Имена кампаний уникализируются через testutil.UniqueEmail чтобы
// тест проходил на любом состоянии БД.
package campaign

import (
	"context"
	"encoding/json"
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

func TestCampaignCRUD(t *testing.T) {
	t.Parallel()

	t.Run("create unauthenticated returns 401", func(t *testing.T) {
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

	t.Run("create brand_manager forbidden", func(t *testing.T) {
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

	t.Run("create empty name returns 422", func(t *testing.T) {
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

	t.Run("create name too long returns 422", func(t *testing.T) {
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

	t.Run("create empty tmaUrl returns 422", func(t *testing.T) {
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

	t.Run("create tmaUrl too long returns 422", func(t *testing.T) {
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

	t.Run("create success returns 201 with id-only payload and writes audit row", func(t *testing.T) {
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

		// id-only payload: only assert a real uuid came back; the full row
		// shape is exercised by the GET /campaigns/{id} t.Run below.
		got := resp.JSON201.Data
		require.NotEqual(t, uuid.Nil, got.Id, "server-stamped uuid must be present")
		testutil.RegisterCampaignCleanup(t, got.Id.String())

		// Audit-row carries the entire *domain.Campaign serialized into
		// new_value with snake_case keys (Boundaries «Always»). Validate the
		// payload shape end-to-end so AC #4 is closed at the e2e layer too.
		entry := testutil.FindAuditEntry(t, c, token, "campaign", got.Id.String(), "campaign_create")
		require.NotNil(t, entry.NewValue, "audit row must carry new_value JSON")
		// NewValue arrives as interface{} from the generated client; round-trip
		// through json.Marshal+Unmarshal lands a typed map for assertions.
		raw, err := json.Marshal(entry.NewValue)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
		require.Equal(t, got.Id.String(), payload["id"])
		require.Equal(t, name, payload["name"])
		require.Equal(t, validTmaURL, payload["tma_url"])
		require.Equal(t, false, payload["is_deleted"])
		require.NotEmpty(t, payload["created_at"])
		require.NotEmpty(t, payload["updated_at"])
	})

	t.Run("get unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		// Generated client without WithAuth — no Authorization header reaches
		// the server, so the auth middleware short-circuits to 401 before the
		// handler runs.
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000001"))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("get brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "HostBrand-"+testutil.UniqueEmail("getmgr"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		// Use any plausible-but-nonexistent uuid — authz must fail before any
		// DB lookup, so the row's existence is irrelevant.
		resp, err := mgrClient.GetCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("get not found returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.GetCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-deadbeef0000"),
			testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
		require.Equal(t, "Кампания не найдена.", resp.JSON404.Error.Message)
	})

	t.Run("get success returns full campaign", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		name := "Promo-" + testutil.UniqueEmail("getok")
		before := time.Now()

		createResp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name:   name,
			TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, createResp.StatusCode())
		require.NotNil(t, createResp.JSON201)
		id := createResp.JSON201.Data.Id
		require.NotEqual(t, uuid.Nil, id)
		testutil.RegisterCampaignCleanup(t, id.String())

		getResp, err := c.GetCampaignWithResponse(context.Background(), id, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)

		got := getResp.JSON200.Data
		require.Equal(t, id, got.Id)
		require.Equal(t, name, got.Name)
		require.Equal(t, validTmaURL, got.TmaUrl)
		require.False(t, got.IsDeleted, "freshly created campaign must be live")
		require.WithinDuration(t, before, got.CreatedAt, time.Minute)
		require.WithinDuration(t, before, got.UpdatedAt, time.Minute)
	})

	t.Run("update unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Promo-" + testutil.UniqueEmail("upunauth"),
				TmaUrl: validTmaURL,
			})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("update brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "HostBrand-"+testutil.UniqueEmail("upmgr"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		// Authz must fail before any DB lookup, so the row's existence is irrelevant.
		resp, err := mgrClient.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Promo-" + testutil.UniqueEmail("upmgrbody"),
				TmaUrl: validTmaURL,
			}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("update not found returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-deadbeef0001"),
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Promo-" + testutil.UniqueEmail("up404"),
				TmaUrl: validTmaURL,
			}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
		require.Equal(t, "Кампания не найдена.", resp.JSON404.Error.Message)
	})

	t.Run("update empty name returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			apiclient.UpdateCampaignJSONRequestBody{Name: "   ", TmaUrl: validTmaURL},
			testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_NAME_REQUIRED", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "Название кампании обязательно")
	})

	t.Run("update name too long returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000006"),
			apiclient.UpdateCampaignJSONRequestBody{Name: strings.Repeat("A", 256), TmaUrl: validTmaURL},
			testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_NAME_TOO_LONG", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "слишком длинное")
	})

	t.Run("update empty tmaUrl returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000007"),
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Promo-" + testutil.UniqueEmail("upemptyurl"),
				TmaUrl: "   ",
			}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_TMA_URL_REQUIRED", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "Ссылка на TMA-страницу обязательна")
	})

	t.Run("update tmaUrl too long returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		resp, err := c.UpdateCampaignWithResponse(context.Background(),
			uuid.MustParse("00000000-0000-0000-0000-000000000008"),
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Promo-" + testutil.UniqueEmail("uplongurl"),
				TmaUrl: strings.Repeat("A", 2049),
			}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_TMA_URL_TOO_LONG", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "слишком длинная")
	})

	t.Run("update name taken returns 409 CAMPAIGN_NAME_TAKEN", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		nameA := "Promo-A-" + testutil.UniqueEmail("upcollideA")
		nameB := "Promo-B-" + testutil.UniqueEmail("upcollideB")

		respA, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name: nameA, TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, respA.StatusCode())
		require.NotNil(t, respA.JSON201)
		idA := respA.JSON201.Data.Id
		testutil.RegisterCampaignCleanup(t, idA.String())

		respB, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name: nameB, TmaUrl: validTmaURL,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, respB.StatusCode())
		require.NotNil(t, respB.JSON201)
		idB := respB.JSON201.Data.Id
		testutil.RegisterCampaignCleanup(t, idB.String())

		// PATCH B name=A — sequential, no race; partial-unique index trips.
		updateResp, err := c.UpdateCampaignWithResponse(context.Background(), idB,
			apiclient.UpdateCampaignJSONRequestBody{Name: nameA, TmaUrl: validTmaURL},
			testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusConflict, updateResp.StatusCode())
		require.NotNil(t, updateResp.JSON409)
		require.Equal(t, "CAMPAIGN_NAME_TAKEN", updateResp.JSON409.Error.Code)
		require.Contains(t, updateResp.JSON409.Error.Message, "Кампания с таким названием")
	})

	t.Run("update success returns 204 and rewrites name/tmaUrl with audit row", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		nameInit := "Promo-Init-" + testutil.UniqueEmail("upok")
		nameNew := "Promo-New-" + testutil.UniqueEmail("upok")
		tmaInit := "https://tma.ugcboost.kz/tz/init"
		tmaNew := "https://tma.ugcboost.kz/tz/new"

		createResp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
			Name: nameInit, TmaUrl: tmaInit,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, createResp.StatusCode())
		require.NotNil(t, createResp.JSON201)
		id := createResp.JSON201.Data.Id
		testutil.RegisterCampaignCleanup(t, id.String())

		updateResp, err := c.UpdateCampaignWithResponse(context.Background(), id,
			apiclient.UpdateCampaignJSONRequestBody{Name: nameNew, TmaUrl: tmaNew},
			testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, updateResp.StatusCode())
		require.Empty(t, updateResp.Body, "204 must have empty body")

		// Re-fetch to confirm the row landed in DB with new values + bumped updated_at.
		getResp, err := c.GetCampaignWithResponse(context.Background(), id, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)
		got := getResp.JSON200.Data
		require.Equal(t, id, got.Id)
		require.Equal(t, nameNew, got.Name)
		require.Equal(t, tmaNew, got.TmaUrl)
		require.False(t, got.IsDeleted)
		require.True(t, got.UpdatedAt.After(got.CreatedAt), "PATCH must bump updated_at past created_at")

		entry := testutil.FindAuditEntry(t, c, token, "campaign", id.String(), "campaign_update")
		require.NotNil(t, entry.OldValue, "audit row must carry old_value JSON")
		require.NotNil(t, entry.NewValue, "audit row must carry new_value JSON")
		oldRaw, err := json.Marshal(entry.OldValue)
		require.NoError(t, err)
		newRaw, err := json.Marshal(entry.NewValue)
		require.NoError(t, err)
		var oldPayload, newPayload map[string]any
		require.NoError(t, json.Unmarshal(oldRaw, &oldPayload))
		require.NoError(t, json.Unmarshal(newRaw, &newPayload))
		require.Equal(t, id.String(), oldPayload["id"])
		require.Equal(t, nameInit, oldPayload["name"])
		require.Equal(t, tmaInit, oldPayload["tma_url"])
		require.Equal(t, false, oldPayload["is_deleted"])
		require.Equal(t, id.String(), newPayload["id"])
		require.Equal(t, nameNew, newPayload["name"])
		require.Equal(t, tmaNew, newPayload["tma_url"])
		require.Equal(t, false, newPayload["is_deleted"])
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
	// start barrier: both goroutines park on <-start, then close(start)
	// releases them simultaneously so partial-unique 23505 is actually
	// exercised. Without this the for-loop launches goroutines sequentially
	// with enough scheduler slack that the first INSERT often commits before
	// the second goroutine even forms its request — turning a "race" test
	// into a sequential 201→409 check.
	start := make(chan struct{})
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
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
				defer winnerLock.Unlock()
				winnerUUID = resp.JSON201.Data.Id
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
	close(start)
	wg.Wait()

	// otherStatus first so a 5xx surfaces immediately in the failure message
	// instead of being masked by the "expected 1, got 0" on ok201.
	require.Zero(t, otherStatus.Load(), "no 5xx / unexpected status under race")
	require.Equal(t, int32(1), ok201.Load(), "exactly one create must succeed")
	require.Equal(t, int32(1), ok409.Load(), "exactly one create must lose the race with CAMPAIGN_NAME_TAKEN")
	require.NotEqual(t, uuid.Nil, winnerUUID)
	testutil.RegisterCampaignCleanup(t, winnerUUID.String())
}
