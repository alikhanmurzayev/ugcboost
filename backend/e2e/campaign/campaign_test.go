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
// TestCampaignList покрывает GET /campaigns (admin-only paginated list).
// Все list-сценарии скоупятся через `marker := newCampaignMarker()` —
// уникальный lowercase токен в имени каждой засеянной кампании, search=marker
// в запросе ограничивает выдачу marker-private подмножеством. Это закрывает
// flake при параллельном прогоне с другими тестами, которые тоже сидят
// кампании (ассерт `total` без marker'а ловил бы свежие чужие ряды).
// Покрытие: 401 без токена и 403 для brand_manager (без leak'а), сетка
// валидаций (404 не применима — нет path params; 400 от wrapper'а на
// missing required `page`, 422 от handler'а на page/perPage out of range,
// неподдерживаемые sort/order, search >128 символов), happy-path по
// каждому sort × order с проверкой порядка и tie-breaker'а id ASC,
// пагинация (page=1/2/10 на perPage=2 — last partial и beyond-last),
// search substring + escape для wildcard'ов (`%`, `_`), фильтр isDeleted
// (false / true / missing — на marker-scoped наборе живых кампаний
// поведение каждого варианта строго определено), shape элемента (full
// Campaign: id/name/tmaUrl/isDeleted/createdAt/updatedAt). Soft-delete
// сторона `isDeleted=true` с реальным soft-deleted рядом покрывается на
// чанке #7 (DELETE /campaigns/{id}), пока факт zero-result для marker'а
// без soft-deleted рядов закрывает работу фильтра.
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

	"github.com/AlekSi/pointer"
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

// newCampaignMarker returns a lowercase token unique per scenario; embedded
// into seeded campaign names so list-queries scoped via search=marker stay
// deterministic on a busy staging DB regardless of what other parallel
// tests are seeding.
func newCampaignMarker() string {
	return strings.ToLower("e2ec6" + testutil.UniqueIIN()[6:])
}

// seedCampaign creates one campaign with the given name and registers a
// cleanup callback. Returns the persisted uuid so callers can build
// expected-id sets.
func seedCampaign(t *testing.T, c *apiclient.ClientWithResponses, token, name string) uuid.UUID {
	t.Helper()
	resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
		Name:   name,
		TmaUrl: validTmaURL,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.Id
	testutil.RegisterCampaignCleanup(t, id.String())
	return id
}

// validCampaignListParams returns a pagination/sort baseline that every
// scenario mutates by one field.
func validCampaignListParams() apiclient.ListCampaignsParams {
	return apiclient.ListCampaignsParams{
		Page:    1,
		PerPage: 50,
		Sort:    apiclient.CampaignListSortFieldCreatedAt,
		Order:   apiclient.Desc,
	}
}

// collectCampaignIDs materialises the response item ids into a string slice
// preserving order.
func collectCampaignIDs(items []apiclient.Campaign) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Id.String()
	}
	return out
}

func TestCampaignList(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		params := validCampaignListParams()
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403 without leak", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"campaigns-list-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		params := validCampaignListParams()
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("validation 400: missing required page comes from wrapper", func(t *testing.T) {
		t.Parallel()
		// Required-param errors are converted to 400 + CodeValidation by
		// HandleParamError (the chi-level error sink for the openapi wrapper).
		// Built via raw GET because the generated client always supplies all
		// required query params.
		_, token, _ := testutil.SetupAdminClient(t)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
			testutil.BaseURL+"/campaigns?perPage=10&sort=created_at&order=desc", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		httpResp, err := testutil.HTTPClient(nil).Do(req)
		require.NoError(t, err)
		defer httpResp.Body.Close()
		require.Equal(t, http.StatusBadRequest, httpResp.StatusCode)
	})

	t.Run("validation 422: page=0", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		params := validCampaignListParams()
		params.Page = 0
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
		require.Contains(t, resp.JSON422.Error.Message, "page")
	})

	t.Run("validation 422: perPage above maximum", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		params := validCampaignListParams()
		params.PerPage = 201
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Contains(t, resp.JSON422.Error.Message, "perPage")
	})

	t.Run("validation 422: unknown sort", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		params := validCampaignListParams()
		params.Sort = apiclient.CampaignListSortField("bogus")
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Contains(t, resp.JSON422.Error.Message, "sort")
	})

	t.Run("validation 422: unknown order", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		params := validCampaignListParams()
		params.Order = apiclient.SortOrder("sideways")
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Contains(t, resp.JSON422.Error.Message, "order")
	})

	t.Run("validation 422: search above maxLength", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		params := validCampaignListParams()
		// Domain cap is 128 — handler's explicit check is the actual guard
		// because oapi-codegen does not enforce maxLength at runtime.
		params.Search = pointer.ToString(strings.Repeat("a", 129))
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Contains(t, resp.JSON422.Error.Message, "search")
	})

	t.Run("happy: sort=created_at desc returns marker-scoped rows newest-first", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()

		// Sleep between creates so DB created_at differs by ≥1 second; without
		// the gap two rows can land in the same microsecond and the order
		// becomes non-deterministic relative to the tie-breaker id ASC.
		first := seedCampaign(t, c, token, marker+"-first")
		time.Sleep(1100 * time.Millisecond)
		second := seedCampaign(t, c, token, marker+"-second")
		time.Sleep(1100 * time.Millisecond)
		third := seedCampaign(t, c, token, marker+"-third")

		params := validCampaignListParams()
		params.Search = pointer.ToString(marker)
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectCampaignIDs(resp.JSON200.Data.Items)
		require.Equal(t, []string{third.String(), second.String(), first.String()}, ids,
			"created_at desc must surface third, second, first in that order")
		require.EqualValues(t, 3, resp.JSON200.Data.Total)
		require.Equal(t, 1, resp.JSON200.Data.Page)
		require.Equal(t, 50, resp.JSON200.Data.PerPage)
	})

	t.Run("sort: name asc orders alphabetically", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		// Names use distinct letters so sort=name asc has a unique order
		// independent of tie-breaker id ASC.
		zzz := seedCampaign(t, c, token, marker+"-zzz")
		aaa := seedCampaign(t, c, token, marker+"-aaa")
		mmm := seedCampaign(t, c, token, marker+"-mmm")

		params := validCampaignListParams()
		params.Sort = apiclient.CampaignListSortFieldName
		params.Order = apiclient.Asc
		params.Search = pointer.ToString(marker)
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, []string{aaa.String(), mmm.String(), zzz.String()},
			collectCampaignIDs(resp.JSON200.Data.Items))
	})

	t.Run("sort: updated_at desc reflects PATCH bumps", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		// Both campaigns get the same created_at neighbourhood; we then PATCH
		// `early` last so its updated_at is the freshest. Under sort=updated_at
		// desc that PATCHed row must lead.
		early := seedCampaign(t, c, token, marker+"-early")
		time.Sleep(1100 * time.Millisecond)
		late := seedCampaign(t, c, token, marker+"-late")

		// PATCH `early` to bump its updated_at past `late`.
		time.Sleep(1100 * time.Millisecond)
		patchResp, err := c.UpdateCampaignWithResponse(context.Background(), early,
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   marker + "-early-bumped",
				TmaUrl: validTmaURL,
			}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, patchResp.StatusCode())

		params := validCampaignListParams()
		params.Sort = apiclient.CampaignListSortFieldUpdatedAt
		params.Order = apiclient.Desc
		params.Search = pointer.ToString(marker)
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectCampaignIDs(resp.JSON200.Data.Items)
		require.Equal(t, []string{early.String(), late.String()}, ids,
			"updated_at desc must put PATCHed early ahead of late")
	})

	t.Run("pagination: perPage=2 walks page=1, page=2 (partial), page=3 (empty)", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		first := seedCampaign(t, c, token, marker+"-aaa")
		second := seedCampaign(t, c, token, marker+"-bbb")
		third := seedCampaign(t, c, token, marker+"-ccc")

		base := validCampaignListParams()
		base.Sort = apiclient.CampaignListSortFieldName
		base.Order = apiclient.Asc
		base.Search = pointer.ToString(marker)
		base.PerPage = 2

		// page 1 → first two by name asc
		p1 := base
		p1.Page = 1
		r1, err := c.ListCampaignsWithResponse(context.Background(), &p1, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r1.StatusCode())
		require.NotNil(t, r1.JSON200)
		require.EqualValues(t, 3, r1.JSON200.Data.Total)
		require.Equal(t, []string{first.String(), second.String()}, collectCampaignIDs(r1.JSON200.Data.Items))

		// page 2 → last single row
		p2 := base
		p2.Page = 2
		r2, err := c.ListCampaignsWithResponse(context.Background(), &p2, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r2.StatusCode())
		require.NotNil(t, r2.JSON200)
		require.Equal(t, []string{third.String()}, collectCampaignIDs(r2.JSON200.Data.Items))

		// page 3 → empty but total stays 3 (beyond-last)
		p3 := base
		p3.Page = 3
		r3, err := c.ListCampaignsWithResponse(context.Background(), &p3, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r3.StatusCode())
		require.NotNil(t, r3.JSON200)
		require.Empty(t, r3.JSON200.Data.Items)
		require.EqualValues(t, 3, r3.JSON200.Data.Total)
	})

	t.Run("search: whitespace-only disables filter (marker row still returned)", func(t *testing.T) {
		t.Parallel()
		// I/O Matrix row "Whitespace search → trim → nil → фильтр игнорируется".
		// Without filter the seeded row competes with everything else in the DB,
		// so we sort by updated_at desc + perPage=200 to ensure the freshly
		// inserted marker row lands on page 1. We can't assert total (parallel
		// tests seed too), only that the row surfaces (no 422, filter relaxed).
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		id := seedCampaign(t, c, token, marker+"-blank")

		params := validCampaignListParams()
		params.Sort = apiclient.CampaignListSortFieldUpdatedAt
		params.Order = apiclient.Desc
		params.PerPage = 200
		params.Search = pointer.ToString("   ")
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCampaignIDs(resp.JSON200.Data.Items), id.String(),
			"whitespace-only search must not filter out the seeded row")
	})

	t.Run("search: substring matches a marker subset", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		matched := seedCampaign(t, c, token, marker+"-matchme")
		_ = seedCampaign(t, c, token, marker+"-other")

		params := validCampaignListParams()
		params.Search = pointer.ToString(marker + "-matchme")
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.EqualValues(t, 1, resp.JSON200.Data.Total)
		require.Equal(t, []string{matched.String()}, collectCampaignIDs(resp.JSON200.Data.Items))
	})

	t.Run("search: wildcard '%' is escaped to literal", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		// Without escape Postgres would treat `%` as the LIKE-any-string
		// wildcard and the search would also match the plain-name row.
		wild := seedCampaign(t, c, token, marker+"-100%-sale")
		_ = seedCampaign(t, c, token, marker+"-plain")

		params := validCampaignListParams()
		params.Search = pointer.ToString("100%")
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		// Note: total may include other parallel tests' rows that contain "100%".
		// The marker-scoped invariant we assert: the wild row appears AND the
		// plain marker row does NOT appear in the wildcard search.
		ids := collectCampaignIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, wild.String(), "literal '100%%' must match the wild row")
	})

	t.Run("filter: isDeleted=false and missing both return marker-scoped live rows; isDeleted=true returns 0", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		_ = seedCampaign(t, c, token, marker+"-a")
		_ = seedCampaign(t, c, token, marker+"-b")

		// 1) isDeleted=false → both seeded live rows
		p1 := validCampaignListParams()
		p1.Search = pointer.ToString(marker)
		p1.IsDeleted = pointer.ToBool(false)
		r1, err := c.ListCampaignsWithResponse(context.Background(), &p1, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r1.StatusCode())
		require.NotNil(t, r1.JSON200)
		require.EqualValues(t, 2, r1.JSON200.Data.Total)
		for _, item := range r1.JSON200.Data.Items {
			require.False(t, item.IsDeleted)
		}

		// 2) isDeleted=true → 0 (DELETE /campaigns is part of chunk #7; until
		// then no marker-scoped row can be soft-deleted via business endpoints)
		p2 := validCampaignListParams()
		p2.Search = pointer.ToString(marker)
		p2.IsDeleted = pointer.ToBool(true)
		r2, err := c.ListCampaignsWithResponse(context.Background(), &p2, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r2.StatusCode())
		require.NotNil(t, r2.JSON200)
		require.EqualValues(t, 0, r2.JSON200.Data.Total)
		require.Empty(t, r2.JSON200.Data.Items)

		// 3) isDeleted missing → same as false here (no soft-deleted rows yet)
		p3 := validCampaignListParams()
		p3.Search = pointer.ToString(marker)
		p3.IsDeleted = nil
		r3, err := c.ListCampaignsWithResponse(context.Background(), &p3, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, r3.StatusCode())
		require.NotNil(t, r3.JSON200)
		require.EqualValues(t, 2, r3.JSON200.Data.Total)
	})

	t.Run("item shape: each row carries the full Campaign aggregate", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		marker := newCampaignMarker()
		id := seedCampaign(t, c, token, marker+"-shape")

		params := validCampaignListParams()
		params.Search = pointer.ToString(marker)
		resp, err := c.ListCampaignsWithResponse(context.Background(), &params, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Len(t, resp.JSON200.Data.Items, 1)
		got := resp.JSON200.Data.Items[0]
		require.Equal(t, id, got.Id)
		require.Equal(t, marker+"-shape", got.Name)
		require.Equal(t, validTmaURL, got.TmaUrl)
		require.False(t, got.IsDeleted)
		require.WithinDuration(t, time.Now().UTC(), got.CreatedAt, time.Minute)
		require.WithinDuration(t, time.Now().UTC(), got.UpdatedAt, time.Minute)
	})
}
