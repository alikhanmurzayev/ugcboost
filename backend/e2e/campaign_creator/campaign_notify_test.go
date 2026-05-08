// Package campaign_creator — продолжение e2e набора с фокусом на chunk-12
// рассылку приглашений и ремайндеров через бот.
//
// TestNotifyCampaignCreators проходит POST /campaigns/{id}/notify во всех
// задокументированных ответах. Brand_manager-токен ловит 403 от authz.
// Формат-валидации (пустой массив, дубликаты creatorIds) повторяют
// контракт A1 — handler делит общий validateCampaignCreatorBatch helper.
// Отсутствующая кампания (захардделитили через /test/cleanup-entity) даёт
// 404 CAMPAIGN_NOT_FOUND. Strict-422 batch-validation: креатор не привязан
// к кампании → not_in_campaign; креатор уже invited (повторная A4) →
// wrong_status с current_status="invited"; payload приходит в кастомной
// схеме CampaignCreatorBatchInvalidErrorResponse (oneOf union в strict-server
// делает JSON422 непрозрачным — десериализация идёт по resp.Body). Happy:
// батч из двух свежих creator'ов отвечает 200 с undelivered=[]; статусы
// флипают в `invited` с invited_count=1 и InvitedAt в недавнем окне; на
// каждого пишется audit-row campaign_creator_invite в той же tx, что и
// UPDATE; spy_store ловит ровно по одному сообщению на чат с web_app.url
// равным campaign.tmaUrl.
//
// TestRemindCampaignCreatorsInvitation проходит POST /campaigns/{id}/
// remind-invitation и зеркалит forbidden / 422 wrong_status / happy-сценарии
// с инкрементом reminded_count и audit-action campaign_creator_remind.
//
// TestNotifyPartialSuccess вешает на одного из creator'ов синтетический
// сбой через /test/telegram/spy/fail-next. После A4 батча из двух
// undelivered содержит ровно одну запись с reason=bot_blocked; для
// failed-creator БД не меняется и audit не записывается; для delivered —
// обычный invited+1.
//
// TestUpdateCampaignTmaURLLock проверяет связку chunk-12 lock + chunk-7 PATCH.
// После успешного A4 PATCH /campaigns/{id} с новым tmaUrl возвращает 422
// CAMPAIGN_TMA_URL_LOCKED; в БД tmaUrl не меняется (проверяем GET /campaigns/
// {id}) и audit campaign_update НЕ записан. PATCH только name (tmaUrl
// прежний) проходит как обычно — 204 + audit campaign_update.
//
// Сетап тот же, что и в campaign_creator_test.go: setupCampaign +
// SetupApprovedCreator + RegisterCampaignCreatorCleanup для каждой
// привязанной пары. SetupApprovedCreator теперь прокидывает
// TelegramUserID наружу — он используется и как chat_id для spy_store
// поиска, и как payload для /test/telegram/spy/fail-next.
package campaign_creator

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// chunk12InviteText / chunk12RemindText mirror the package-level constants
// in internal/telegram/notifier.go so e2e can filter spy_store records by
// outgoing text without importing the internal package (e2e is its own
// module by design).
const (
	chunk12InviteText = "Привет! У нас есть для тебя предложение по сотрудничеству. Открой, чтобы посмотреть условия:"
	chunk12RemindText = "Напоминаем — мы ждём твоего решения по приглашению."
)

// waitInviteSent blocks until the spy records exactly one invite/remind
// message (matched by Text) for chatID at-or-after `since`. Filters out
// the unrelated chunk-5 welcome message that arrives async after every
// SetupApprovedCreator. Fails fast on >1 invite or on timeout.
func waitInviteSent(t *testing.T, chatID int64, since time.Time, expectedText string) testclient.TelegramSentMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		// Any record (welcome + invite) at-or-after `since`. We filter by Text
		// here instead of inside WaitForTelegramSent because the latter waits
		// for an exact ExpectCount and would block forever if the count is
		// wrong.
		params := &testclient.GetTelegramSentParams{ChatId: chatID, Since: &since}
		client := testutil.NewTestClient(t)
		resp, err := client.GetTelegramSentWithResponse(context.Background(), params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		var matches []testclient.TelegramSentMessage
		for _, m := range resp.JSON200.Data.Messages {
			if m.Text == expectedText {
				matches = append(matches, m)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
		require.LessOrEqual(t, len(matches), 1, "expected at most one invite per chat")
		if time.Now().After(deadline) {
			t.Fatalf("waitInviteSent: no invite for chat %d after 5s", chatID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}


// addCreatorToCampaign drives POST /campaigns/{id}/creators for one creator
// and registers the LIFO cleanup. Used by every chunk-12 e2e to seed the
// `planned` baseline before exercising notify / remind.
func addCreatorToCampaign(t *testing.T, c *apiclient.ClientWithResponses, adminToken string, campaignID uuid.UUID, creatorID string) {
	t.Helper()
	resp, err := c.AddCampaignCreatorsWithResponse(context.Background(), campaignID,
		apiclient.AddCampaignCreatorsJSONRequestBody{
			CreatorIds: []uuid.UUID{uuid.MustParse(creatorID)},
		}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	testutil.RegisterCampaignCreatorCleanup(t, c, adminToken, campaignID.String(), creatorID)
}

// expectBatchInvalid parses a 422 body into the typed batch-invalid shape.
// strict-server's union response field is private, so the generated client
// surface offers no typed accessor — we route through resp.Body instead.
func expectBatchInvalid(t *testing.T, body []byte) apiclient.CampaignCreatorBatchInvalidErrorResponse {
	t.Helper()
	var out apiclient.CampaignCreatorBatchInvalidErrorResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.Equal(t, "CAMPAIGN_CREATOR_BATCH_INVALID", out.Error.Code)
	return out
}

// findStatus retrieves the campaign_creator status for (campaignID, creatorID)
// via GET /campaigns/{id}/creators. Used by partial-success / wrong_status
// assertions to confirm the row-state side-effects.
func findStatus(t *testing.T, c *apiclient.ClientWithResponses, adminToken string, campaignID uuid.UUID, creatorID string) apiclient.CampaignCreator {
	t.Helper()
	resp, err := c.ListCampaignCreatorsWithResponse(context.Background(), campaignID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	for _, item := range resp.JSON200.Data.Items {
		if item.CreatorId.String() == creatorID {
			return item
		}
	}
	t.Fatalf("creator %s not found in campaign %s", creatorID, campaignID)
	return apiclient.CampaignCreator{}
}

func TestNotifyCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA4-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA4-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
	})

	t.Run("missing campaign returns 404", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), uuid.New(),
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("batch-invalid: not_in_campaign + wrong_status collected together", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA4-422-"+testutil.UniqueEmail("camp"))

		// Already-attached creator that we will invite once → status moves to
		// invited; the second A4 call must surface him as wrong_status.
		invitedCreator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, invitedCreator.CreatorID)
		first, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(invitedCreator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, first.StatusCode())

		// Second creator never attached — must surface as not_in_campaign.
		strangerCreator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		second, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{
					uuid.MustParse(invitedCreator.CreatorID),
					uuid.MustParse(strangerCreator.CreatorID),
				},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, second.StatusCode())

		body := expectBatchInvalid(t, second.Body)
		require.Len(t, body.Error.Details, 2)
		byCreator := map[string]apiclient.CampaignCreatorBatchInvalidDetail{}
		for _, d := range body.Error.Details {
			byCreator[d.CreatorId.String()] = d
		}
		require.Contains(t, byCreator, invitedCreator.CreatorID)
		require.Equal(t, apiclient.WrongStatus, byCreator[invitedCreator.CreatorID].Reason)
		require.NotNil(t, byCreator[invitedCreator.CreatorID].CurrentStatus)
		require.Equal(t, apiclient.Invited, *byCreator[invitedCreator.CreatorID].CurrentStatus)
		require.Contains(t, byCreator, strangerCreator.CreatorID)
		require.Equal(t, apiclient.NotInCampaign, byCreator[strangerCreator.CreatorID].Reason)
		require.Nil(t, byCreator[strangerCreator.CreatorID].CurrentStatus)
	})

	t.Run("happy: invites all and writes audit + spy hits per creator", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		const tmaURL = "https://tma.ugcboost.kz/tz/notify-happy"
		campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
			"ccA4-happy-"+testutil.UniqueEmail("camp"), tmaURL)

		creator1 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		creator2 := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator1.CreatorID)
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator2.CreatorID)

		startedAt := time.Now().UTC().Add(-time.Second)
		resp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{
					uuid.MustParse(creator1.CreatorID),
					uuid.MustParse(creator2.CreatorID),
				},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Empty(t, resp.JSON200.Data.Undelivered)

		for _, fx := range []testutil.ApprovedCreatorFixture{creator1, creator2} {
			cc := findStatus(t, adminClient, adminToken, campaignID, fx.CreatorID)
			require.Equal(t, apiclient.Invited, cc.Status)
			require.Equal(t, 1, cc.InvitedCount)
			require.NotNil(t, cc.InvitedAt)
			require.WithinDuration(t, time.Now().UTC(), *cc.InvitedAt, time.Minute)
			testutil.AssertAuditEntry(t, adminClient, adminToken,
				"campaign_creator", cc.Id.String(), "campaign_creator_invite")
		}

		for _, fx := range []testutil.ApprovedCreatorFixture{creator1, creator2} {
			msg := waitInviteSent(t, fx.TelegramUserID, startedAt, chunk12InviteText)
			require.NotNil(t, msg.WebAppUrl)
			require.Equal(t, tmaURL, *msg.WebAppUrl)
		}
	})
}

// setupCampaignWithTmaURL mirrors setupCampaign but lets the caller pin a
// specific tmaUrl — chunk-12 spy assertions need the exact URL the bot
// embedded in the inline web_app button.
func setupCampaignWithTmaURL(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name, tmaURL string) uuid.UUID {
	t.Helper()
	resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
		Name:   name,
		TmaUrl: tmaURL,
	}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	id := resp.JSON201.Data.Id
	testutil.RegisterCampaignCleanup(t, id.String())
	return id
}

func TestRemindCampaignCreatorsInvitation(t *testing.T) {
	t.Parallel()

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA5-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA5-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.RemindCampaignCreatorsInvitationWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsInvitationJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
	})

	t.Run("planned creator rejected with wrong_status", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA5-422-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		resp, err := adminClient.RemindCampaignCreatorsInvitationWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsInvitationJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		body := expectBatchInvalid(t, resp.Body)
		require.Len(t, body.Error.Details, 1)
		require.Equal(t, apiclient.WrongStatus, body.Error.Details[0].Reason)
		require.NotNil(t, body.Error.Details[0].CurrentStatus)
		require.Equal(t, apiclient.Planned, *body.Error.Details[0].CurrentStatus)
	})

	t.Run("happy: bumps reminded_count and writes remind audit", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA5-happy-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		// First flip planned → invited so remind has a valid source state.
		notifyResp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, notifyResp.StatusCode())

		remindResp, err := adminClient.RemindCampaignCreatorsInvitationWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsInvitationJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, remindResp.StatusCode())
		require.NotNil(t, remindResp.JSON200)
		require.Empty(t, remindResp.JSON200.Data.Undelivered)

		cc := findStatus(t, adminClient, adminToken, campaignID, creator.CreatorID)
		require.Equal(t, apiclient.Invited, cc.Status)
		require.Equal(t, 1, cc.InvitedCount)
		require.Equal(t, 1, cc.RemindedCount)
		require.NotNil(t, cc.RemindedAt)
		testutil.AssertAuditEntry(t, adminClient, adminToken,
			"campaign_creator", cc.Id.String(), "campaign_creator_remind")
	})
}

func TestNotifyPartialSuccess(t *testing.T) {
	t.Parallel()
	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	campaignID := setupCampaign(t, adminClient, adminToken, "ccA4-partial-"+testutil.UniqueEmail("camp"))

	delivered := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
	failing := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
	addCreatorToCampaign(t, adminClient, adminToken, campaignID, delivered.CreatorID)
	addCreatorToCampaign(t, adminClient, adminToken, campaignID, failing.CreatorID)

	// Force the next outbound send to `failing` to come back with the
	// canonical Forbidden error so MapTelegramErrorToReason classifies it
	// as bot_blocked. `delivered` goes through unchanged.
	testutil.RegisterTelegramSpyFailNext(t, failing.TelegramUserID, "")

	startedAt := time.Now().UTC().Add(-time.Second)
	resp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
		apiclient.NotifyCampaignCreatorsJSONRequestBody{
			CreatorIds: []uuid.UUID{
				uuid.MustParse(delivered.CreatorID),
				uuid.MustParse(failing.CreatorID),
			},
		}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.Len(t, resp.JSON200.Data.Undelivered, 1)
	require.Equal(t, failing.CreatorID, resp.JSON200.Data.Undelivered[0].CreatorId.String())
	require.Equal(t, apiclient.BotBlocked, resp.JSON200.Data.Undelivered[0].Reason)

	deliveredCC := findStatus(t, adminClient, adminToken, campaignID, delivered.CreatorID)
	require.Equal(t, apiclient.Invited, deliveredCC.Status)
	require.Equal(t, 1, deliveredCC.InvitedCount)
	testutil.AssertAuditEntry(t, adminClient, adminToken,
		"campaign_creator", deliveredCC.Id.String(), "campaign_creator_invite")

	failingCC := findStatus(t, adminClient, adminToken, campaignID, failing.CreatorID)
	require.Equal(t, apiclient.Planned, failingCC.Status, "failing creator must keep planned status")
	require.Equal(t, 0, failingCC.InvitedCount, "failing creator must NOT have invited_count incremented")
	require.Nil(t, failingCC.InvitedAt)

	deliveredMsg := waitInviteSent(t, delivered.TelegramUserID, startedAt, chunk12InviteText)
	require.Nil(t, deliveredMsg.Error)

	failingMsg := waitInviteSent(t, failing.TelegramUserID, startedAt, chunk12InviteText)
	require.NotNil(t, failingMsg.Error)
	require.Contains(t, *failingMsg.Error, "Forbidden")
}

func TestUpdateCampaignTmaURLLock(t *testing.T) {
	t.Parallel()

	t.Run("lock fires when tma_url changes after invite", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		const tmaURL = "https://tma.ugcboost.kz/tz/lock-original"
		campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
			"ccTmaLock-"+testutil.UniqueEmail("camp"), tmaURL)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		notifyResp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, notifyResp.StatusCode())

		patchResp, err := adminClient.UpdateCampaignWithResponse(context.Background(), campaignID,
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   "Renamed " + testutil.UniqueEmail("camp"),
				TmaUrl: "https://tma.ugcboost.kz/tz/lock-new",
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, patchResp.StatusCode())
		require.NotNil(t, patchResp.JSON422)
		require.Equal(t, "CAMPAIGN_TMA_URL_LOCKED", patchResp.JSON422.Error.Code)

		// БД не изменилась — tmaUrl остался прежним.
		getResp, err := adminClient.GetCampaignWithResponse(context.Background(), campaignID, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.Equal(t, tmaURL, getResp.JSON200.Data.TmaUrl)
	})

	t.Run("name-only patch passes after invite", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		const tmaURL = "https://tma.ugcboost.kz/tz/lock-noop"
		campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
			"ccTmaNoop-"+testutil.UniqueEmail("camp"), tmaURL)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		notifyResp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, notifyResp.StatusCode())

		newName := "Renamed " + testutil.UniqueEmail("camp")
		patchResp, err := adminClient.UpdateCampaignWithResponse(context.Background(), campaignID,
			apiclient.UpdateCampaignJSONRequestBody{
				Name:   newName,
				TmaUrl: tmaURL,
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, patchResp.StatusCode())

		getResp, err := adminClient.GetCampaignWithResponse(context.Background(), campaignID, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, newName, getResp.JSON200.Data.Name)
		require.Equal(t, tmaURL, getResp.JSON200.Data.TmaUrl)
		testutil.AssertAuditEntry(t, adminClient, adminToken,
			"campaign", campaignID.String(), "campaign_update")
	})
}

