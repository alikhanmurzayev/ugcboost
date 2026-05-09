// Package creator_applications — E2E HTTP-поверхность опционального параметра
// `campaignIds` у POST /creators/applications/{id}/approve. Расширение
// approve-ручки sequential-add'ами в выбранные кампании после tx1 и Telegram-
// notify, с handler-уровневой валидацией (cap=20, dedupe) плюс pre-validation
// существования и `is_deleted=false` через CampaignService.AssertActiveCampaigns.
//
// TestApproveWithCampaigns собирает заявку в `moderation` через
// SetupCreatorApplicationInModeration, заводит N кампаний админом и проходит
// через все ветки валидации/потока. Happy_with_two_campaigns одобряет заявку
// с двумя свежесозданными кампаниями: assert'ы ловят 200, новый `creatorId`,
// смену статуса заявки на `approved`, ровно одну audit-row
// `creator_application_approve` и по одной `campaign_creator_add` на каждую
// добавленную пару (campaign, creator); `campaign_creators` через
// ListCampaignCreators отдаёт строки в `planned`. Сценарий validation_too_many
// шлёт 21 случайный UUID — handler ловит до открытия любой транзакции, ответ
// 422 `CAMPAIGN_IDS_TOO_MANY`, заявка остаётся в `moderation` без новых
// audit-rows. Validation_duplicates повторяет один и тот же UUID — 422
// `CAMPAIGN_IDS_DUPLICATES`, без писем. Pre_validation_missing_id ставит
// сначала валидную кампанию, затем добавляет заведомо несуществующий UUID:
// service approve вызвать не должен (creator не создаётся, заявка остаётся
// в `moderation`), ответ — 422 `CAMPAIGN_NOT_AVAILABLE_FOR_ADD`. Soft-deleted
// под-сценарий не покрыт: бизнес-API soft-delete'а кампаний пока нет, а
// тестового рукоятка под него спецой не предусмотрена — единый код покрывает
// оба кейса в коде, и существование-проверка протестирована через
// non-existent. Mid-cycle race (campaign A добавлена, B удалена между
// pre-check и циклом) тем же образом фиксируется как ограничение: через HTTP
// его не достичь, юнит-тест ApproveApplication first-fail-stop его покрывает.
//
// Cleanup строго LIFO: campaign_creators (если успели вставиться в happy-path,
// удаляются вместе с creator hard-cleanup'ом и cleanup'ом кампаний из-за
// ON DELETE на FK кампании отсутствует — отдельные RegisterCampaignCreatorCleanup
// зарегистрированы в happy-сценарии после parent'ов). Cleanup кампаний и
// креатора через test-API — POST /test/cleanup-entity. Все t.Run параллельны,
// каждый создаёт собственный набор данных через UniqueEmail / UniqueIIN.
package creator_applications_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

const (
	auditActionCampaignCreatorAdd  = "campaign_creator_add"
	auditEntityTypeCampaignCreator = "campaign_creator"
)

// Error codes mirror domain.Code* — backend/e2e is a separate Go module that
// cannot import internal/, so the canonical strings live as package-local
// constants. Sharing one definition per file lets the assertions point at the
// constant rather than retyping the literal in every t.Run.
const (
	codeCampaignIdsTooMany         = "CAMPAIGN_IDS_TOO_MANY"
	codeCampaignIdsDuplicates      = "CAMPAIGN_IDS_DUPLICATES"
	codeCampaignNotAvailableForAdd = "CAMPAIGN_NOT_AVAILABLE_FOR_ADD"
)

// setupApproveCampaign creates a campaign owned by adminToken and registers
// a LIFO cleanup. Returns the campaign UUID. Mirrors setupCampaign in
// campaign_creator_test.go — kept local because Go test packages do not share
// helpers across folders.
func setupApproveCampaign(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name string) uuid.UUID {
	t.Helper()
	resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
		Name:   name,
		TmaUrl: testutil.FreshValidTmaURL(),
	}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode(),
		"setupApproveCampaign: expected 201, got %d body=%s", resp.StatusCode(), resp.Body)
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.Id
	require.NotEqual(t, uuid.Nil, id)
	testutil.RegisterCampaignCleanup(t, id.String())
	return id
}

func TestApproveWithCampaigns(t *testing.T) {
	t.Parallel()

	t.Run("happy path: approve attaches creator to two campaigns sequentially", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "approve_camps_" + testutil.UniqueIIN()[6:],
					Verification: testutil.VerificationAutoIG},
			},
		})
		c := testutil.NewAPIClient(t)

		campA := setupApproveCampaign(t, c, fx.AdminToken,
			"approve-camps-A-"+testutil.UniqueEmail("camp"))
		campB := setupApproveCampaign(t, c, fx.AdminToken,
			"approve-camps-B-"+testutil.UniqueEmail("camp"))

		appUUID := uuid.MustParse(fx.ApplicationID)
		body := apiclient.CreatorApprovalInput{
			CampaignIds: &[]uuid.UUID{campA, campB},
		}
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			body, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equalf(t, http.StatusOK, resp.StatusCode(), "body=%s", resp.Body)
		require.NotNil(t, resp.JSON200)
		creatorID := resp.JSON200.Data.CreatorId
		require.NotEqual(t, uuid.Nil, creatorID)
		// Order matters: campaign_creators FK to creators has no ON DELETE, so
		// the rows must drop before the creator. RegisterCampaignCreatorCleanup
		// is LIFO and fires first because it is registered AFTER the creator.
		testutil.RegisterCreatorCleanup(t, creatorID.String())
		testutil.RegisterCampaignCreatorCleanup(t, c, fx.AdminToken, campA.String(), creatorID.String())
		testutil.RegisterCampaignCreatorCleanup(t, c, fx.AdminToken, campB.String(), creatorID.String())

		// Application moved to approved.
		detail := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Approved, detail.Status)

		// One creator_application_approve audit row.
		auditApprove := testutil.FindAuditEntry(t, c, fx.AdminToken,
			auditEntityTypeCreatorApplicationApprove, fx.ApplicationID,
			auditActionCreatorApplicationApprove)
		require.NotNil(t, auditApprove.NewValue)

		// Roster has both rows in `planned`.
		listResp, err := c.ListCampaignCreatorsWithResponse(context.Background(), campA,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		itemsA := filterRosterByCreator(listResp.JSON200.Data.Items, creatorID)
		require.Len(t, itemsA, 1)
		require.Equal(t, apiclient.Planned, itemsA[0].Status)

		listResp, err = c.ListCampaignCreatorsWithResponse(context.Background(), campB,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		itemsB := filterRosterByCreator(listResp.JSON200.Data.Items, creatorID)
		require.Len(t, itemsB, 1)
		require.Equal(t, apiclient.Planned, itemsB[0].Status)

		// One campaign_creator_add audit per (campaign, creator) pair. Parse
		// new_value into a typed shape: substring matching would tolerate a
		// regression where creatorId migrates into a differently-named field
		// (e.g. "creator" or "creator_uuid") since the UUID still appears
		// somewhere in the JSON.
		assertCampaignCreatorAuditPayload(t, c, fx.AdminToken,
			itemsA[0].Id.String(), campA, creatorID)
		assertCampaignCreatorAuditPayload(t, c, fx.AdminToken,
			itemsB[0].Id.String(), campB, creatorID)
	})

	t.Run("> 20 campaignIds returns 422 CAMPAIGN_IDS_TOO_MANY; application stays in moderation", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "approve_too_many_" + testutil.UniqueIIN()[6:],
					Verification: testutil.VerificationAutoIG},
			},
		})
		c := testutil.NewAPIClient(t)

		ids := make([]uuid.UUID, 21)
		for i := range ids {
			ids[i] = uuid.New()
		}
		body := apiclient.CreatorApprovalInput{CampaignIds: &ids}
		appUUID := uuid.MustParse(fx.ApplicationID)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			body, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, codeCampaignIdsTooMany, resp.JSON422.Error.Code)

		stillModeration := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Moderation, stillModeration.Status,
			"validation must reject before any creator/audit row is written")
	})

	t.Run("duplicate campaignIds returns 422 CAMPAIGN_IDS_DUPLICATES", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "approve_dups_" + testutil.UniqueIIN()[6:],
					Verification: testutil.VerificationAutoIG},
			},
		})
		c := testutil.NewAPIClient(t)
		camp := setupApproveCampaign(t, c, fx.AdminToken,
			"approve-dups-camp-"+testutil.UniqueEmail("camp"))

		body := apiclient.CreatorApprovalInput{
			CampaignIds: &[]uuid.UUID{camp, camp},
		}
		appUUID := uuid.MustParse(fx.ApplicationID)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			body, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, codeCampaignIdsDuplicates, resp.JSON422.Error.Code)

		stillModeration := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Moderation, stillModeration.Status)
	})

	t.Run("non-existent campaign returns 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD; service approve not called", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "approve_missing_" + testutil.UniqueIIN()[6:],
					Verification: testutil.VerificationAutoIG},
			},
		})
		c := testutil.NewAPIClient(t)
		camp := setupApproveCampaign(t, c, fx.AdminToken,
			"approve-missing-camp-"+testutil.UniqueEmail("camp"))

		// Mix of one valid and one impossible-to-resolve UUID — pre-validation
		// must reject before opening tx1.
		body := apiclient.CreatorApprovalInput{
			CampaignIds: &[]uuid.UUID{camp, uuid.New()},
		}
		appUUID := uuid.MustParse(fx.ApplicationID)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			body, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, codeCampaignNotAvailableForAdd, resp.JSON422.Error.Code)

		stillModeration := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Moderation, stillModeration.Status,
			"pre-validation must short-circuit before tx1 — no creator row created")

		// And the valid campaign got nothing — handler bailed before service approve.
		listResp, err := c.ListCampaignCreatorsWithResponse(context.Background(), camp,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		require.Empty(t, listResp.JSON200.Data.Items, "no campaign_creators row must exist for the valid campaign")
	})
}

// filterRosterByCreator returns the subset of campaign-creator rows attached
// to the given creator. Used by the happy-path assertion to find the row this
// test wrote without coupling on insertion order across parallel tests.
func filterRosterByCreator(items []apiclient.CampaignCreator, creatorID uuid.UUID) []apiclient.CampaignCreator {
	out := make([]apiclient.CampaignCreator, 0, 1)
	for _, item := range items {
		if item.CreatorId == creatorID {
			out = append(out, item)
		}
	}
	return out
}

// assertCampaignCreatorAuditPayload pulls the audit row matching the given
// (campaign_creator id, action) pair, decodes new_value into a typed shape
// (campaignId, creatorId, status — the rest of the row is dynamic and not
// pinned here) and asserts the snake_case JSON contract.
func assertCampaignCreatorAuditPayload(
	t *testing.T,
	c *apiclient.ClientWithResponses,
	adminToken string,
	campaignCreatorID string,
	expectedCampaignID, expectedCreatorID uuid.UUID,
) {
	t.Helper()
	audit := testutil.FindAuditEntry(t, c, adminToken,
		auditEntityTypeCampaignCreator, campaignCreatorID,
		auditActionCampaignCreatorAdd)
	require.NotNil(t, audit.NewValue)

	payload, err := json.Marshal(audit.NewValue)
	require.NoError(t, err)
	var typed struct {
		ID         string `json:"id"`
		CampaignID string `json:"campaign_id"`
		CreatorID  string `json:"creator_id"`
		Status     string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(payload, &typed))
	require.Equal(t, campaignCreatorID, typed.ID)
	require.Equal(t, expectedCampaignID.String(), typed.CampaignID)
	require.Equal(t, expectedCreatorID.String(), typed.CreatorID)
	require.Equal(t, string(apiclient.Planned), typed.Status)
}
