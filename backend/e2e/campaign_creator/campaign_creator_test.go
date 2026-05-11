// Package campaign_creator — E2E тесты HTTP-поверхности /campaigns/{id}/creators.
//
// TestAddCampaignCreators проходит POST /campaigns/{id}/creators во всех
// задокументированных ответах. Без токена raw HTTP отдаёт 401 — public-доступ
// к админ-каталогу состава кампаний закрыт middleware'ом ещё до handler'а.
// Brand_manager-токен ловит 403 FORBIDDEN от authz-сервиса (без leak'а
// существования кампании). Сетка валидаций для admin-токена: пустой массив
// creatorIds → 422 CAMPAIGN_CREATOR_IDS_REQUIRED, дубликаты в batch'е → 422
// CAMPAIGN_CREATOR_IDS_DUPLICATES, несуществующая кампания → 404
// CAMPAIGN_NOT_FOUND, несуществующий creatorId → 422 CREATOR_NOT_FOUND с
// откатом всего батча, повторное добавление того же креатора → 422
// CREATOR_ALREADY_IN_CAMPAIGN (translation pgErr 23505 на partial unique
// `campaign_creators_campaign_creator_unique` в repo). Happy-path: батч из
// двух свежих креаторов отвечает 201 + items=2 со status=planned, нулевыми
// счётчиками и NULL-таймстемпами; для каждого креатора в audit_logs
// появляется отдельная строка campaign_creator_add в той же транзакции
// (per-creator, не per-batch — стандарт `backend-transactions.md` § Аудит-лог).
//
// TestRemoveCampaignCreator проходит DELETE /campaigns/{id}/creators/{creatorId}.
// Без токена → 401, brand_manager → 403, несуществующая кампания → 404
// CAMPAIGN_NOT_FOUND, существующая кампания + не привязанный креатор → 404
// CAMPAIGN_CREATOR_NOT_FOUND. 422-ветка для status=agreed закрывается на
// chunk #14 — до тех пор бизнес-флоу для перехода в `agreed` не существует.
// Happy-path: добавляем креатора через A1, удаляем через A2 — 204 без тела,
// в audit_logs появляется campaign_creator_remove с полным snapshot'ом
// в old_value (NewValue=nil — запись удалена).
//
// TestPatchCampaignCreator проходит PATCH /campaigns/{id}/creators/{creatorId}
// — admin-only universal partial-update участия креатора в кампании. Без
// токена middleware отдаёт 401 ещё до handler'а; brand_manager-токен ловит
// 403 FORBIDDEN от authz-сервиса. Сетка валидаций для admin-токена: пустой
// body (объект без полей) → 422 CAMPAIGN_CREATOR_PATCH_EMPTY (handler-уровень
// защита от случайного no-op вызова); несуществующая кампания → 404
// CAMPAIGN_NOT_FOUND; существующая кампания + не привязанный креатор → 404
// CAMPAIGN_CREATOR_NOT_FOUND; participation в статусе ≠ `signed` (наш кейс —
// свежий `planned` creator) и попытка ticketSent=true → 422
// CAMPAIGN_CREATOR_TICKET_SENT_BAD_STATUS. Happy-path: signed creator
// (через SetupCampaignWithSigningCreator + TrustMe webhook signed →
// cc.status='signed') получает PATCH ticketSent=true → 200, ticketSentAt
// близко к now(), audit-row `campaign_creator.ticket_sent` появляется в
// той же транзакции с old.ticket_sent_at=null и new.ticket_sent_at=<ts>;
// повторный PATCH ticketSent=true без изменений — 200 no-op, повторно
// audit-row не пишется (idempotent admin clicks не засоряют журнал);
// PATCH ticketSent=false — 200, ticketSentAt=null, новая audit-row с
// обратным diff'ом (old=<ts>, new=null).
//
// TestListCampaignCreators проходит GET /campaigns/{id}/creators (без
// пагинации — admin UI показывает весь roster одной выдачей). Без токена →
// 401, brand_manager → 403, несуществующая кампания → 404; пустой список
// для свежей кампании → 200 + items=[]; happy: 2 attached → 200 + items в
// порядке created_at ASC, id ASC (стабильный порядок repo-уровня).
//
// Сетап компонуется через testutil.SetupAdminClient + SetupBrand +
// SetupManagerWithLogin для 403-кейсов и testutil.SetupApprovedCreator для
// привязываемых креаторов; кампании создаются через POST /campaigns и
// автоматически снимаются после теста через POST /test/cleanup-entity при
// E2E_CLEANUP=true (дефолт). Привязки campaign_creators не каскадятся при
// hard-delete кампании (FK без ON DELETE CASCADE) — поэтому каждая
// фактически вставленная пара регистрируется через
// testutil.RegisterCampaignCreatorCleanup, чтобы LIFO-cleanup снимал
// привязку перед родительскими кампанией и креатором.
package campaign_creator

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// defaultCreatorOpts mirrors the fixture from creators/list_test.go: one
// auto-verified Instagram social plus an unverified TikTok handle so the
// approve flow has exactly the verification it needs.
func defaultCreatorOpts(suffix string) testutil.CreatorApplicationFixture {
	return testutil.CreatorApplicationFixture{
		Socials: []testutil.SocialFixture{
			{Platform: string(apiclient.Instagram), Handle: strings.ToLower("aidana_ig_" + suffix), Verification: testutil.VerificationAutoIG},
			{Platform: string(apiclient.Tiktok), Handle: strings.ToLower("aidana_tt_" + suffix), Verification: testutil.VerificationNone},
		},
	}
}

// setupCampaign creates a campaign owned by the given admin and registers the
// LIFO cleanup. Returns the campaign UUID.
func setupCampaign(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name string) uuid.UUID {
	t.Helper()
	resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
		Name:   name,
		TmaUrl: testutil.FreshValidTmaURL(),
	}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode(),
		"setupCampaign: expected 201, got %d body=%s", resp.StatusCode(), resp.Body)
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.Id
	require.NotEqual(t, uuid.Nil, id)
	testutil.RegisterCampaignCleanup(t, id.String())
	return id
}

func TestAddCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		// Use a real admin only to seed a campaign so the request doesn't 404
		// before it hits the auth middleware. The actual A1 call goes via
		// PostRaw with no Authorization to reach the 401 path.
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-unauth-"+testutil.UniqueEmail("cc"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp := testutil.PostRaw(t, "/campaigns/"+campaignID.String()+"/creators",
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			})
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA1-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("empty creatorIds returns 422 CAMPAIGN_CREATOR_IDS_REQUIRED", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-empty-"+testutil.UniqueEmail("camp"))

		resp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_CREATOR_IDS_REQUIRED", resp.JSON422.Error.Code)
	})

	t.Run("duplicate creatorIds returns 422 CAMPAIGN_CREATOR_IDS_DUPLICATES", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-dups-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creatorUUID := uuid.MustParse(creator.CreatorID)

		resp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{creatorUUID, creatorUUID},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_CREATOR_IDS_DUPLICATES", resp.JSON422.Error.Code)
	})

	t.Run("non-existent campaign returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), uuid.New(),
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("non-existent creator returns 422 CREATOR_NOT_FOUND with full rollback", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-bogus-"+testutil.UniqueEmail("camp"))
		// Real creator first, bogus second. If rollback is broken, the
		// valid INSERT survives — empty-list assertion below catches it.
		// A [bogus]-only batch couldn't distinguish "rolled back" from
		// "nothing ever inserted".
		validCreator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{
					uuid.MustParse(validCreator.CreatorID),
					uuid.New(),
				},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_NOT_FOUND", resp.JSON422.Error.Code)

		// Rollback assertion: even though the first creatorId was valid,
		// no row must survive since the batch failed strict-422.
		listResp, err := adminClient.ListCampaignCreatorsWithResponse(context.Background(), campaignID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		require.Empty(t, listResp.JSON200.Data.Items, "valid id must NOT have been inserted — rollback contract")
	})

	t.Run("re-add returns 422 CREATOR_ALREADY_IN_CAMPAIGN", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-reAdd-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creatorUUID := uuid.MustParse(creator.CreatorID)

		// First add succeeds.
		first, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, first.StatusCode())
		testutil.RegisterCampaignCreatorCleanup(t, adminClient, adminToken, campaignID.String(), creator.CreatorID)

		// Second add of the same creator hits the partial unique → 422.
		second, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, second.StatusCode())
		require.NotNil(t, second.JSON422)
		require.Equal(t, "CREATOR_ALREADY_IN_CAMPAIGN", second.JSON422.Error.Code)
	})

	t.Run("happy: batch of 2 returns 201 with planned items and writes per-creator audit", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA1-happy-"+testutil.UniqueEmail("camp"))
		creator1 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creator2 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		c1UUID := uuid.MustParse(creator1.CreatorID)
		c2UUID := uuid.MustParse(creator2.CreatorID)

		adminUserID := getAdminUserID(t, adminClient, adminToken)

		resp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{c1UUID, c2UUID}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		// Register cleanup as soon as the response shape lets us — BEFORE any
		// shape/contract asserts. A failing require below mid-test would
		// otherwise leak campaign_creators rows that block parent campaign /
		// creator hard-delete via the FK without ON DELETE CASCADE.
		if resp.JSON201 != nil {
			for _, item := range resp.JSON201.Data.Items {
				testutil.RegisterCampaignCreatorCleanup(t, adminClient, adminToken,
					campaignID.String(), item.CreatorId.String())
			}
		}
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.NotNil(t, resp.JSON201)
		items := resp.JSON201.Data.Items
		require.Len(t, items, 2)

		// Pair returned items by creatorId — order matches input by repo
		// contract (one Add per id), but assert the shape per row to keep
		// the test resilient to per-creator id reordering bugs.
		byCreator := map[uuid.UUID]apiclient.CampaignCreator{}
		for _, item := range items {
			byCreator[item.CreatorId] = item
			require.NotEqual(t, uuid.Nil, item.Id, "server-stamped row id must be present")
			require.Equal(t, campaignID, item.CampaignId)
			require.Equal(t, apiclient.Planned, item.Status)
			require.Equal(t, 0, item.InvitedCount)
			require.Equal(t, 0, item.RemindedCount)
			require.Nil(t, item.InvitedAt)
			require.Nil(t, item.RemindedAt)
			require.Nil(t, item.DecidedAt)
			require.WithinDuration(t, time.Now().UTC(), item.CreatedAt, time.Minute)
			require.WithinDuration(t, time.Now().UTC(), item.UpdatedAt, time.Minute)
		}
		_, ok1 := byCreator[c1UUID]
		_, ok2 := byCreator[c2UUID]
		require.True(t, ok1, "creator1 must appear in the response")
		require.True(t, ok2, "creator2 must appear in the response")

		// Audit must contain one campaign_creator_add per creator, with the
		// new_value JSON encoding the persisted snapshot, actor_id pinned
		// to the admin user_id (AC).
		for _, item := range items {
			entry := testutil.FindAuditEntry(t, adminClient, adminToken,
				"campaign_creator", item.Id.String(), "campaign_creator_add")
			require.NotNil(t, entry)
			require.Nil(t, entry.OldValue, "OldValue must be nil for add")
			require.NotNil(t, entry.NewValue, "NewValue must carry the snapshot")
			require.NotNil(t, entry.ActorId, "ActorId must be the admin user_id")
			require.Equal(t, adminUserID, *entry.ActorId)
			payload, err := json.Marshal(entry.NewValue)
			require.NoError(t, err)
			require.Contains(t, string(payload), item.Id.String())
			require.Contains(t, string(payload), item.CreatorId.String())
		}
	})
}

// getAdminUserID resolves the seeded admin's user id via GET /auth/me. The
// generated SetupAdminClient returns email but not id; this round-trip is
// the cheapest stable way to get it for actor_id audit assertions.
func getAdminUserID(t *testing.T, c *apiclient.ClientWithResponses, adminToken string) string {
	t.Helper()
	resp, err := c.GetMeWithResponse(context.Background(), testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Id
}

func TestRemoveCampaignCreator(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA2-unauth-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		// Raw HTTP DELETE without auth — bypass the generated client which would
		// require WithAuth on every request.
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
			testutil.BaseURL+"/campaigns/"+campaignID.String()+"/creators/"+creator.CreatorID, nil)
		require.NoError(t, err)
		resp, err := testutil.HTTPClient(nil).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA2-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA2-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.RemoveCampaignCreatorWithResponse(context.Background(), campaignID,
			uuid.MustParse(creator.CreatorID), testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("non-existent campaign returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.RemoveCampaignCreatorWithResponse(context.Background(),
			uuid.New(), uuid.MustParse(creator.CreatorID), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("non-attached creator returns 404 CAMPAIGN_CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA2-detached-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.RemoveCampaignCreatorWithResponse(context.Background(),
			campaignID, uuid.MustParse(creator.CreatorID), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_CREATOR_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("happy: add then remove returns 204 and writes audit", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		adminUserID := getAdminUserID(t, adminClient, adminToken)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA2-happy-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creatorUUID := uuid.MustParse(creator.CreatorID)

		// Register cleanup BEFORE the explicit Remove below — guards against
		// require-failure between Add and Remove leaving an orphan row that
		// blocks parent campaign / creator hard-delete via the FK.
		testutil.RegisterCampaignCreatorCleanup(t, adminClient, adminToken,
			campaignID.String(), creator.CreatorID)

		addResp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, addResp.StatusCode())
		require.NotNil(t, addResp.JSON201)
		require.Len(t, addResp.JSON201.Data.Items, 1)
		ccID := addResp.JSON201.Data.Items[0].Id

		removeResp, err := adminClient.RemoveCampaignCreatorWithResponse(context.Background(),
			campaignID, creatorUUID, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, removeResp.StatusCode())
		require.Empty(t, removeResp.Body, "204 must not carry a body")

		// Audit row campaign_creator_remove with full snapshot in old_value
		// (new_value is null since the row was deleted), actor_id = admin (AC).
		entry := testutil.FindAuditEntry(t, adminClient, adminToken,
			"campaign_creator", ccID.String(), "campaign_creator_remove")
		require.NotNil(t, entry)
		require.Nil(t, entry.NewValue, "NewValue must be nil for remove")
		require.NotNil(t, entry.OldValue, "OldValue must carry the snapshot")
		require.NotNil(t, entry.ActorId, "ActorId must be the admin user_id")
		require.Equal(t, adminUserID, *entry.ActorId)
		payload, err := json.Marshal(entry.OldValue)
		require.NoError(t, err)
		require.Contains(t, string(payload), ccID.String())
		require.Contains(t, string(payload), creator.CreatorID)

		// List must reflect the removal.
		listResp, err := adminClient.ListCampaignCreatorsWithResponse(context.Background(),
			campaignID, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		require.Empty(t, listResp.JSON200.Data.Items)
	})
}

func TestListCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA3-unauth-"+testutil.UniqueEmail("camp"))

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
			testutil.BaseURL+"/campaigns/"+campaignID.String()+"/creators", nil)
		require.NoError(t, err)
		resp, err := testutil.HTTPClient(nil).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA3-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA3-403-camp-"+testutil.UniqueEmail("camp"))

		resp, err := mgrClient.ListCampaignCreatorsWithResponse(context.Background(), campaignID,
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("non-existent campaign returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)

		resp, err := adminClient.ListCampaignCreatorsWithResponse(context.Background(),
			uuid.New(), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("empty roster returns 200 with empty items", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA3-empty-"+testutil.UniqueEmail("camp"))

		resp, err := adminClient.ListCampaignCreatorsWithResponse(context.Background(), campaignID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Empty(t, resp.JSON200.Data.Items)
	})

	t.Run("happy: 2 attached creators returned in created_at ASC, id ASC order", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA3-happy-"+testutil.UniqueEmail("camp"))
		creator1 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creator2 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		addResp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.AddCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{
					uuid.MustParse(creator1.CreatorID),
					uuid.MustParse(creator2.CreatorID),
				},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		// Register cleanup BEFORE require asserts to guard row leaks on flake.
		if addResp.JSON201 != nil {
			for _, item := range addResp.JSON201.Data.Items {
				testutil.RegisterCampaignCreatorCleanup(t, adminClient, adminToken,
					campaignID.String(), item.CreatorId.String())
			}
		}
		require.Equal(t, http.StatusCreated, addResp.StatusCode())
		require.NotNil(t, addResp.JSON201)
		require.Len(t, addResp.JSON201.Data.Items, 2)

		listResp, err := adminClient.ListCampaignCreatorsWithResponse(context.Background(), campaignID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		items := listResp.JSON200.Data.Items
		require.Len(t, items, 2)

		// Repo contract: ORDER BY created_at ASC, id ASC. The two rows were
		// inserted in the same transaction so created_at is identical; the
		// tie-breaker is id ASC.
		require.True(t, items[0].CreatedAt.Before(items[1].CreatedAt) ||
			items[0].CreatedAt.Equal(items[1].CreatedAt),
			"items[0].CreatedAt must be <= items[1].CreatedAt")
		if items[0].CreatedAt.Equal(items[1].CreatedAt) {
			require.True(t, items[0].Id.String() < items[1].Id.String(),
				"on equal created_at the tie-breaker is id ASC")
		}

		// Per-item full contract: every required field carries a server-stamped
		// value, every nullable timestamp is nil on `planned` rows, both counters
		// are zero, and timestamps are recent. Pairing by CreatorId guards
		// against a list-by-wrong-campaign regression that would still return
		// 2 rows but with the wrong creators.
		c1UUID := uuid.MustParse(creator1.CreatorID)
		c2UUID := uuid.MustParse(creator2.CreatorID)
		byCreator := map[uuid.UUID]apiclient.CampaignCreator{}
		for _, item := range items {
			byCreator[item.CreatorId] = item
			require.NotEqual(t, uuid.Nil, item.Id, "server-stamped row id must be present")
			require.Equal(t, campaignID, item.CampaignId)
			require.Equal(t, apiclient.Planned, item.Status)
			require.Equal(t, 0, item.InvitedCount)
			require.Equal(t, 0, item.RemindedCount)
			require.Nil(t, item.InvitedAt)
			require.Nil(t, item.RemindedAt)
			require.Nil(t, item.DecidedAt)
			require.WithinDuration(t, time.Now().UTC(), item.CreatedAt, time.Minute)
			require.WithinDuration(t, time.Now().UTC(), item.UpdatedAt, time.Minute)
		}
		_, ok1 := byCreator[c1UUID]
		_, ok2 := byCreator[c2UUID]
		require.True(t, ok1, "creator1 must appear in the listed roster")
		require.True(t, ok2, "creator2 must appear in the listed roster")
	})
}

func TestPatchCampaignCreator(t *testing.T) {
	t.Parallel()

	patchURL := func(campaignID, creatorID string) string {
		return testutil.BaseURL + "/campaigns/" + campaignID + "/creators/" + creatorID
	}

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccPatch-unauth-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch,
			patchURL(campaignID.String(), creator.CreatorID),
			strings.NewReader(`{"ticketSent":true}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := testutil.HTTPClient(nil).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccPatch-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccPatch-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignID, uuid.MustParse(creator.CreatorID),
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("empty body returns 422 CAMPAIGN_CREATOR_PATCH_EMPTY", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccPatch-empty-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignID, uuid.MustParse(creator.CreatorID),
			apiclient.PatchCampaignCreatorJSONRequestBody{}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_CREATOR_PATCH_EMPTY", resp.JSON422.Error.Code)
	})

	t.Run("non-existent campaign returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.PatchCampaignCreatorWithResponse(context.Background(),
			uuid.New(), uuid.MustParse(creator.CreatorID),
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("non-attached creator returns 404 CAMPAIGN_CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccPatch-detached-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignID, uuid.MustParse(creator.CreatorID),
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_CREATOR_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("non-signed status returns 422 TICKET_SENT_BAD_STATUS", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccPatch-badstatus-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creatorUUID := uuid.MustParse(creator.CreatorID)
		testutil.RegisterCampaignCreatorCleanup(t, adminClient, adminToken,
			campaignID.String(), creator.CreatorID)

		// Add as `planned` (default initial state — not `signed`).
		addResp, err := adminClient.AddCampaignCreatorsWithResponse(context.Background(),
			campaignID, apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, addResp.StatusCode())

		resp, err := adminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignID, creatorUUID,
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CAMPAIGN_CREATOR_TICKET_SENT_BAD_STATUS", resp.JSON422.Error.Code)
	})

	t.Run("happy: signed → set, no-op repeat, unset, audit diff", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		adminUserID := getAdminUserID(t, fx.AdminClient, fx.AdminToken)

		// Flip to signed via TrustMe webhook (status=3 happy-signed payload).
		token := testutil.TrustMeWebhookToken(t)
		status, body := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookSignedPayload(fx.TrustMeDocumentID), token)
		require.Equalf(t, 200, status, "webhook signed: %s", string(body))

		campaignUUID := uuid.MustParse(fx.CampaignID)
		creatorUUID := uuid.MustParse(fx.CreatorID)

		// SET — ticketSentAt populated near now(); audit row appears.
		setResp, err := fx.AdminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignUUID, creatorUUID,
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equalf(t, http.StatusOK, setResp.StatusCode(), "set: %s", string(setResp.Body))
		require.NotNil(t, setResp.JSON200)
		require.NotNil(t, setResp.JSON200.Data.TicketSentAt, "ticketSentAt must be populated after set")
		require.WithinDuration(t, time.Now().UTC(), *setResp.JSON200.Data.TicketSentAt, time.Minute)
		firstStamp := *setResp.JSON200.Data.TicketSentAt

		entrySet := testutil.FindAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.ticket_sent")
		require.NotNil(t, entrySet, "set must write audit row")
		require.Equal(t, "campaign_creator.ticket_sent", entrySet.Action)
		require.Equal(t, "campaign_creator", entrySet.EntityType)
		require.NotNil(t, entrySet.EntityId)
		require.Equal(t, fx.CampaignCreatorID, *entrySet.EntityId)
		require.NotNil(t, entrySet.ActorId, "audit must record admin actor_id from middleware")
		require.Equal(t, adminUserID, *entrySet.ActorId)
		require.NotNil(t, entrySet.OldValue, "audit set must carry old snapshot")
		require.NotNil(t, entrySet.NewValue, "audit set must carry new snapshot")

		// Audit payload — parse as map so we can assert exact field shapes
		// without importing internal/domain (e2e is a separate module).
		// `ticket_sent_at` MUST be present in both snapshots (no omitempty in
		// the domain struct) and MUST be null in OldValue / non-null in NewValue.
		oldMap := auditValueMap(t, entrySet.OldValue)
		newMap := auditValueMap(t, entrySet.NewValue)
		require.Contains(t, oldMap, "ticket_sent_at", "OldValue must carry the field explicitly")
		require.Nil(t, oldMap["ticket_sent_at"], "OldValue.ticket_sent_at must be null pre-set")
		require.Contains(t, newMap, "ticket_sent_at")
		require.NotNil(t, newMap["ticket_sent_at"], "NewValue.ticket_sent_at must be a timestamp string")

		// NO-OP REPEAT — same value, server returns row unchanged and skips audit.
		noopResp, err := fx.AdminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignUUID, creatorUUID,
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(true)},
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, noopResp.StatusCode())
		require.NotNil(t, noopResp.JSON200.Data.TicketSentAt)
		require.Equal(t, firstStamp, *noopResp.JSON200.Data.TicketSentAt,
			"no-op must return the same ticketSentAt — no re-stamp on idempotent click")

		auditCount := countAuditEntries(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.ticket_sent")
		require.Equal(t, 1, auditCount, "no-op must not write a second audit row")

		// UNSET — back to null; second audit row appears with reverse diff.
		unsetResp, err := fx.AdminClient.PatchCampaignCreatorWithResponse(context.Background(),
			campaignUUID, creatorUUID,
			apiclient.PatchCampaignCreatorJSONRequestBody{TicketSent: pointer.ToBool(false)},
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, unsetResp.StatusCode())
		require.Nil(t, unsetResp.JSON200.Data.TicketSentAt, "ticketSentAt must be cleared after unset")

		auditCount = countAuditEntries(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.ticket_sent")
		require.Equal(t, 2, auditCount, "unset must write a second audit row")

		// ListAuditLogs sorts created_at DESC, so the freshest entry is the
		// just-written unset row — assert its actor + payload directly.
		entryUnset := testutil.FindAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.ticket_sent")
		require.NotNil(t, entryUnset, "unset must write audit row")
		require.NotNil(t, entryUnset.ActorId)
		require.Equal(t, adminUserID, *entryUnset.ActorId, "unset audit actor_id must match admin")
		unsetOldMap := auditValueMap(t, entryUnset.OldValue)
		unsetNewMap := auditValueMap(t, entryUnset.NewValue)
		require.NotNil(t, unsetOldMap["ticket_sent_at"], "OldValue.ticket_sent_at must be the previously-set timestamp")
		require.Contains(t, unsetNewMap, "ticket_sent_at")
		require.Nil(t, unsetNewMap["ticket_sent_at"], "NewValue.ticket_sent_at must be null after unset")
	})
}

// auditValueMap decodes an audit_logs JSON snapshot into a generic map so
// individual fields can be asserted without importing internal/domain
// (e2e is a separate module). Accepts the openapi-generated interface{} from
// AuditLogEntry.OldValue / NewValue.
func auditValueMap(t *testing.T, raw interface{}) map[string]any {
	t.Helper()
	require.NotNil(t, raw)
	payload, err := json.Marshal(raw)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(payload, &m))
	return m
}

// countAuditEntries returns the number of audit rows matching
// (entityType, entityID, action). Caller pins entityID to a fresh fixture
// row, so the per-row count never realistically nears the per_page cap —
// but we still assert `len < per_page` to fail loudly if a future test
// floods the same fixture id with audit rows and silently truncates.
func countAuditEntries(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string,
) int {
	t.Helper()
	const perPage = 100
	resp, err := c.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			Page:       pointer.ToInt(1),
			PerPage:    pointer.ToInt(perPage),
			EntityType: pointer.ToString(entityType),
			EntityId:   pointer.ToString(entityID),
			Action:     pointer.ToString(action),
		},
		testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	got := len(resp.JSON200.Data.Logs)
	require.Lessf(t, got, perPage,
		"audit count reached per_page=%d cap — assertion would silently truncate; paginate or narrow filters",
		perPage)
	return got
}
