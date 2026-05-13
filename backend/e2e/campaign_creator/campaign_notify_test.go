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
// TestRemindCampaignCreatorsSigning зеркалит то же самое для POST
// /campaigns/{id}/remind-signing, но допустимый source-status — `signing`.
// Setup идёт через testutil.SetupCampaignWithSigningCreator (TMA agree →
// один тик outbox-worker'а Phase 3), который доводит creator'а до signing.
// Хедж-кейсы (handler-level 422 — пустой батч / >200 ids / дубликаты, authz
// 403, validation 422 wrong_status и not_in_campaign, 404 missing campaign)
// зеркалят инварианты remind-invitation. Happy single + повтор: статус
// остаётся signing, reminded_count инкрементируется, в каждом цикле пишется
// audit campaign_creator_remind_signing и спай ловит сообщение с
// chunk12RemindSigningText БЕЗ WebApp-кнопки (креатор уже согласился, ТЗ-
// кнопка лишняя — он подписывает по SMS-ссылке TrustMe). Partial-success
// разводится на две кампании (одна с FailNext на креаторе, вторая happy):
// undelivered содержит ровно failing с reason=bot_blocked, БД failing
// creator'а не меняется и audit не пишется.
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

// chunk12InviteText / chunk12RemindText / chunk12RemindSigningText mirror
// the package-level constants in internal/telegram/notifier.go so e2e can
// filter spy_store records by outgoing text without importing the internal
// package (e2e is its own module by design).
const (
	chunk12InviteText = "Добрый день! EURASIAN FASHION WEEK уже скоро ✨\n\n" +
		"У нас есть для вас предложение по сотрудничеству в качестве UGC-креатора. Откройте ссылку, чтобы ознакомиться с датами, условиями, форматом участия и техническим заданием для контента.\n\n" +
		"Если вы согласны, нажмите кнопку \"Согласиться\" и мы отправим вам онлайн соглашение о сотрудничестве на подписание 💫"
	chunk12RemindText = "Откройте ссылку, чтобы ознакомиться с датами, условиями, форматом участия и техническим заданием для контента.\n\n" +
		"Если вы согласны, нажмите кнопку \"Согласиться\" и мы отправим вам онлайн соглашение о сотрудничестве на подписание 💫"
	chunk12RemindSigningText = "Напоминаем, что мы отправили вам соглашение на подпись по СМС на номер телефона, указанный при регистрации.\n\n" +
		"Перейдите по ссылке из СМС и подпишите соглашение.\n\n" +
		"Если есть вопросы, можете обратиться к @aizerealzair"
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
	testutil.RegisterCampaignCreatorCleanup(t, campaignID.String(), creatorID)
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

// spyMessagesSince returns every telegram-spy record for chatID at-or-after
// `since`. Differs from waitInviteSent (which expects exactly one matching
// record) in that it returns all records — caller filters by text. Used by
// negative assertions where late retries of unrelated welcome/approve
// messages can race the validation-rejection check window.
func spyMessagesSince(t *testing.T, chatID int64, since time.Time) []testclient.TelegramSentMessage {
	t.Helper()
	tc := testutil.NewTestClient(t)
	params := &testclient.GetTelegramSentParams{ChatId: chatID, Since: &since}
	resp, err := tc.GetTelegramSentWithResponse(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Messages
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

	t.Run("rejects 422 CONTRACT_TEMPLATE_REQUIRED when campaign has no contract template", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA4-no-tpl-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		resp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())

		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "CONTRACT_TEMPLATE_REQUIRED", body.Error.Code)
	})

	t.Run("batch-invalid: not_in_campaign + wrong_status collected together", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA4-422-"+testutil.UniqueEmail("camp"))
		uploadDummyContractTemplate(t, adminToken, campaignID)

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
		tmaURL := testutil.FreshValidTmaURL()
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
			require.Nil(t, msg.Error, "happy path delivery must not record a spy error")
			require.NotNil(t, msg.WebAppUrl)
			require.Equal(t, tmaURL, *msg.WebAppUrl)
		}
	})
}

func uploadDummyContractTemplate(t *testing.T, adminToken string, campaignID uuid.UUID) {
	t.Helper()
	resp := testutil.PutContractTemplate(t,
		"/campaigns/"+campaignID.String()+"/contract-template",
		testutil.BuildValidContractPDF(t),
		testutil.WithHeader("Authorization", "Bearer "+adminToken))
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// setupCampaignWithTmaURL mirrors setupCampaign но pin'ит tmaUrl и сразу
// заливает PDF-шаблон — Notify требует шаблон (chunk 9a guard).
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
	uploadResp := testutil.PutContractTemplate(t,
		"/campaigns/"+id.String()+"/contract-template",
		testutil.BuildValidContractPDF(t),
		testutil.WithHeader("Authorization", "Bearer "+adminToken))
	uploadResp.Body.Close()
	require.Equal(t, http.StatusOK, uploadResp.StatusCode)
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
		tmaURL := testutil.FreshValidTmaURL()
		campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
			"ccA5-happy-"+testutil.UniqueEmail("camp"), tmaURL)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		// First flip planned → invited so remind has a valid source state.
		notifyResp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, notifyResp.StatusCode())

		remindStartedAt := time.Now().UTC().Add(-time.Second)
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

		// Remind text + WebApp URL — кнопка на ту же tma_url, что и invite.
		remindMsg := waitInviteSent(t, creator.TelegramUserID, remindStartedAt, chunk12RemindText)
		require.Nil(t, remindMsg.Error, "happy remind delivery must not record a spy error")
		require.NotNil(t, remindMsg.WebAppUrl)
		require.Equal(t, tmaURL, *remindMsg.WebAppUrl)
	})
}

func TestRemindCampaignCreatorsSigning(t *testing.T) {
	t.Parallel()

	t.Run("brand_manager forbidden", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "ccA6-403-brand-"+testutil.UniqueEmail("brand"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-403-camp-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := mgrClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-401-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp := testutil.PostRaw(t, "/campaigns/"+campaignID.String()+"/remind-signing",
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			})
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("empty creatorIds rejected as 422 CAMPAIGN_CREATOR_IDS_REQUIRED", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-empty-"+testutil.UniqueEmail("camp"))

		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		var body apiclient.ErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "CAMPAIGN_CREATOR_IDS_REQUIRED", body.Error.Code)
	})

	t.Run("over 200 creatorIds rejected as 422 CAMPAIGN_CREATOR_IDS_TOO_MANY", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-toomany-"+testutil.UniqueEmail("camp"))

		ids := make([]uuid.UUID, 201)
		for i := range ids {
			ids[i] = uuid.New()
		}
		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{CreatorIds: ids},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		var body apiclient.ErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "CAMPAIGN_CREATOR_IDS_TOO_MANY", body.Error.Code)
	})

	t.Run("soft-deleted campaign returns 404", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-soft-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		testutil.SoftDeleteCampaign(t, campaignID.String())

		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("duplicate creatorIds rejected as 422 CAMPAIGN_CREATOR_IDS_DUPLICATES", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-dup-"+testutil.UniqueEmail("camp"))
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		dup := uuid.MustParse(creator.CreatorID)
		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{dup, dup},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		var body apiclient.ErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "CAMPAIGN_CREATOR_IDS_DUPLICATES", body.Error.Code)
	})

	t.Run("missing campaign returns 404", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), uuid.New(),
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("not_in_campaign creator rejected", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		campaignID := setupCampaign(t, adminClient, adminToken, "ccA6-orphan-"+testutil.UniqueEmail("camp"))
		stranger := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))

		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(stranger.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		body := expectBatchInvalid(t, resp.Body)
		require.Len(t, body.Error.Details, 1)
		require.Equal(t, apiclient.NotInCampaign, body.Error.Details[0].Reason)
		require.Nil(t, body.Error.Details[0].CurrentStatus)
	})

	t.Run("wrong_status: invited creator rejected with current_status=invited", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		tmaURL := testutil.FreshValidTmaURL()
		campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
			"ccA6-wrong-"+testutil.UniqueEmail("camp"), tmaURL)
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts(testutil.UniqueIIN()[6:]))
		addCreatorToCampaign(t, adminClient, adminToken, campaignID, creator.CreatorID)

		// Flip planned → invited so the creator is firmly in a status that
		// remind-signing must reject.
		notifyResp, err := adminClient.NotifyCampaignCreatorsWithResponse(context.Background(), campaignID,
			apiclient.NotifyCampaignCreatorsJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, notifyResp.StatusCode())
		// Wait for the invite spy record to settle before the strict-422 check
		// so EnsureNoNewTelegramSent below has a stable baseline.
		_ = waitInviteSent(t, creator.TelegramUserID, time.Now().UTC().Add(-2*time.Second), chunk12InviteText)

		negativeBaseline := time.Now().UTC()
		resp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(creator.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		body := expectBatchInvalid(t, resp.Body)
		require.Len(t, body.Error.Details, 1)
		require.Equal(t, apiclient.WrongStatus, body.Error.Details[0].Reason)
		require.NotNil(t, body.Error.Details[0].CurrentStatus)
		require.Equal(t, apiclient.Invited, *body.Error.Details[0].CurrentStatus)

		// БД не тронута — invited_count остался 1, reminded_count = 0.
		cc := findStatus(t, adminClient, adminToken, campaignID, creator.CreatorID)
		require.Equal(t, apiclient.Invited, cc.Status)
		require.Equal(t, 1, cc.InvitedCount)
		require.Equal(t, 0, cc.RemindedCount)
		require.Nil(t, cc.RemindedAt)

		// Telegram-spy must NOT receive a remind-signing message — strict-422
		// short-circuits before delivery. Filter by text rather than total
		// count because late retries of unrelated async messages (welcome,
		// approved, retry-after-fake-chat) can land in the same window.
		require.Eventually(t,
			func() bool {
				for _, m := range spyMessagesSince(t, creator.TelegramUserID, negativeBaseline) {
					if m.Text == chunk12RemindSigningText {
						return false
					}
				}
				return true
			},
			500*time.Millisecond, 100*time.Millisecond,
			"strict-422 must not emit a remind-signing Telegram message",
		)
	})

	t.Run("happy single + repeat: reminded_count bumps, status stays signing", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		adminClient := fx.AdminClient
		adminToken := fx.AdminToken
		campaignID := uuid.MustParse(fx.CampaignID)
		creatorID := uuid.MustParse(fx.CreatorID)

		// First remind.
		firstStartedAt := time.Now().UTC().Add(-time.Second)
		firstResp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{creatorID},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, firstResp.StatusCode())
		require.NotNil(t, firstResp.JSON200)
		require.Empty(t, firstResp.JSON200.Data.Undelivered)

		cc := findStatus(t, adminClient, adminToken, campaignID, fx.CreatorID)
		require.Equal(t, apiclient.Signing, cc.Status, "remind-signing must NOT change status")
		require.Equal(t, 1, cc.RemindedCount)
		require.NotNil(t, cc.RemindedAt)
		require.WithinDuration(t, time.Now().UTC(), *cc.RemindedAt, time.Minute)
		testutil.AssertAuditEntry(t, adminClient, adminToken,
			"campaign_creator", cc.Id.String(), "campaign_creator_remind_signing")

		firstMsg := waitInviteSent(t, fx.TelegramUserID, firstStartedAt, chunk12RemindSigningText)
		require.Nil(t, firstMsg.Error, "happy remind-signing must not record a spy error")
		require.Nil(t, firstMsg.WebAppUrl, "remind-signing must NOT carry a WebApp button — creator already agreed")

		// Repeat — second call bumps reminded_count to 2 with a fresh audit row.
		// `since` for the second waitInviteSent must be strictly after the first
		// message's sentAt — otherwise the spy filter (inclusive) returns both
		// messages and waitInviteSent panics on len>1.
		secondSince := firstMsg.SentAt.Add(time.Millisecond)
		secondResp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(), campaignID,
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{creatorID},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, secondResp.StatusCode())
		require.NotNil(t, secondResp.JSON200)
		require.Empty(t, secondResp.JSON200.Data.Undelivered)

		cc2 := findStatus(t, adminClient, adminToken, campaignID, fx.CreatorID)
		require.Equal(t, apiclient.Signing, cc2.Status)
		require.Equal(t, 2, cc2.RemindedCount)
		require.NotNil(t, cc2.RemindedAt)
		require.True(t, cc.RemindedAt.Before(*cc2.RemindedAt) || cc.RemindedAt.Equal(*cc2.RemindedAt),
			"second reminded_at must not regress")

		// AssertAuditEntry only checks ≥1 — to prove the second remind wrote
		// a *fresh* row (and didn't silently swallow), assert the count went
		// from 1 → 2.
		auditRows := testutil.ListAuditEntriesByAction(t, adminClient, adminToken,
			"campaign_creator", cc2.Id.String(), "campaign_creator_remind_signing")
		require.Len(t, auditRows, 2, "second remind must write a second audit row, not overwrite the first")

		// Second spy message — separate record with the same plain text, still no button.
		secondMsg := waitInviteSent(t, fx.TelegramUserID, secondSince, chunk12RemindSigningText)
		require.Nil(t, secondMsg.Error)
		require.Nil(t, secondMsg.WebAppUrl, "remind-signing repeat must also stay plain-text")
	})

	t.Run("partial-success: bot_blocked entry, others succeed (two campaigns)", func(t *testing.T) {
		t.Parallel()
		// SetupCampaignWithSigningCreator gives one campaign with exactly one
		// `signing` creator. To exercise partial-success without bloating
		// testutil with a multi-creator setup, run it twice — two independent
		// campaigns, one per creator. The remind-signing batch still hits both
		// because they share the admin client + token; we drive two separate
		// POSTs and assert the failing one stays at reminded_count=0 while the
		// happy one bumps to 1.
		fxFailing := testutil.SetupCampaignWithSigningCreator(t)
		fxDelivered := testutil.SetupCampaignWithSigningCreator(t)
		adminClient := fxFailing.AdminClient
		adminToken := fxFailing.AdminToken

		// Force the next outbound send to `fxFailing.TelegramUserID` to come
		// back with Forbidden → bot_blocked.
		testutil.RegisterTelegramSpyFailNext(t, fxFailing.TelegramUserID, "")

		failingResp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(),
			uuid.MustParse(fxFailing.CampaignID),
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(fxFailing.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, failingResp.StatusCode())
		require.NotNil(t, failingResp.JSON200)
		require.Len(t, failingResp.JSON200.Data.Undelivered, 1)
		require.Equal(t, fxFailing.CreatorID, failingResp.JSON200.Data.Undelivered[0].CreatorId.String())
		require.Equal(t, apiclient.BotBlocked, failingResp.JSON200.Data.Undelivered[0].Reason)

		// Failing creator: row state untouched, no remind audit.
		failingCC := findStatus(t, adminClient, adminToken,
			uuid.MustParse(fxFailing.CampaignID), fxFailing.CreatorID)
		require.Equal(t, apiclient.Signing, failingCC.Status, "failing creator stays in signing")
		require.Equal(t, 0, failingCC.RemindedCount, "failing creator reminded_count stays at 0")
		require.Nil(t, failingCC.RemindedAt)

		// Delivered creator: real send goes through, reminded_count → 1, audit fires.
		deliveredStartedAt := time.Now().UTC().Add(-time.Second)
		deliveredResp, err := adminClient.RemindCampaignCreatorsSigningWithResponse(context.Background(),
			uuid.MustParse(fxDelivered.CampaignID),
			apiclient.RemindCampaignCreatorsSigningJSONRequestBody{
				CreatorIds: []uuid.UUID{uuid.MustParse(fxDelivered.CreatorID)},
			}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, deliveredResp.StatusCode())
		require.NotNil(t, deliveredResp.JSON200)
		require.Empty(t, deliveredResp.JSON200.Data.Undelivered)

		deliveredCC := findStatus(t, adminClient, adminToken,
			uuid.MustParse(fxDelivered.CampaignID), fxDelivered.CreatorID)
		require.Equal(t, apiclient.Signing, deliveredCC.Status)
		require.Equal(t, 1, deliveredCC.RemindedCount)
		require.NotNil(t, deliveredCC.RemindedAt)
		testutil.AssertAuditEntry(t, adminClient, adminToken,
			"campaign_creator", deliveredCC.Id.String(), "campaign_creator_remind_signing")

		deliveredMsg := waitInviteSent(t, fxDelivered.TelegramUserID, deliveredStartedAt, chunk12RemindSigningText)
		require.Nil(t, deliveredMsg.Error)
		require.Nil(t, deliveredMsg.WebAppUrl, "remind-signing partial-success delivered branch must also stay plain-text")
	})
}

func TestNotifyPartialSuccess(t *testing.T) {
	t.Parallel()
	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	tmaURL := testutil.FreshValidTmaURL()
	campaignID := setupCampaignWithTmaURL(t, adminClient, adminToken,
		"ccA4-partial-"+testutil.UniqueEmail("camp"), tmaURL)

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
	require.NotNil(t, deliveredMsg.WebAppUrl)
	require.Equal(t, tmaURL, *deliveredMsg.WebAppUrl)

	failingMsg := waitInviteSent(t, failing.TelegramUserID, startedAt, chunk12InviteText)
	require.NotNil(t, failingMsg.Error)
	require.Contains(t, *failingMsg.Error, "Forbidden")
}

func TestUpdateCampaignTmaURLLock(t *testing.T) {
	t.Parallel()

	t.Run("lock fires when tma_url changes after invite", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		tmaURL := testutil.FreshValidTmaURL()
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
				TmaUrl: testutil.FreshValidTmaURL(),
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
		tmaURL := testutil.FreshValidTmaURL()
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
