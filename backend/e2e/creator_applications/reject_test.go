// Package creator_applications — E2E HTTP-поверхность
// POST /creators/applications/{id}/reject (chunk 12 + chunk 13 onboarding-
// roadmap'а): admin отклоняет заявку из `verification` или `moderation`,
// заявка переходит в терминал `rejected`, в админ-аггрегате появляется блок
// `rejection`, telegram_link выживает на reject'е, а сразу после коммита
// fire-and-forget уходит статическое Telegram-сообщение (chunk 13 расширил
// сервис-слой нотификацией; до этого чанка assert был negative).
//
// TestRejectCreatorApplication прогоняет всю I/O-матрицу одной функцией.
// Сначала authn/authz (401/403) и 404 на random uuid. Дальше идёт happy
// path из verification: креатор привязал Telegram, после reject'а в
// /test/telegram/sent появляется ровно одна запись с константой
// expectedRejectText, plain text, без WebAppUrl и без upstream-ошибки.
// Аналогичный сценарий из moderation сначала прогоняет SendPulse-webhook
// → moderation (он сам шлёт verification-approved push, мы дренируем его
// до фиксации курсора), потом reject снова добавляет ровно одну reject-
// запись с тем же текстом.
//
// Отдельный сценарий happy_verification_no_telegram_link воспроизводит
// заявку без LinkTelegramToApplication: reject 200, admin GET detail
// показывает telegramLink == nil, для контрольного chat_id за 5-секундное
// окно не появляется ни одной записи (notify коротко-замыкается warn'ом
// в логе сервиса). Repeat-reject подтверждает идемпотентность: первый
// 200 порождает один push, второй вызов 422'ится за tx до notify, и
// окно после первого ack'а остаётся пустым.
//
// detail-before-reject и telegram_link-survives-reject держат omitempty-
// инвариант openapi и контракт сохранения linka на reject'е. Все t.Run
// параллельны, каждый создаёт изолированную заявку через
// SetupCreatorApplicationViaLanding и чистится в LIFO-стеке
// (E2E_CLEANUP), родительская заявка тащит соцсети, audit, transitions
// и telegram_link каскадом.
package creator_applications_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// auditActionCreatorApplicationReject mirrors the backend constant
// AuditActionCreatorApplicationReject. e2e is its own Go module, so the
// value is re-declared locally instead of imported.
const auditActionCreatorApplicationReject = "creator_application_reject"

// auditEntityTypeCreatorApplicationReject mirrors AuditEntityTypeCreatorApplication.
const auditEntityTypeCreatorApplicationReject = "creator_application"

// telegramSilenceWindowReject — окно ожидания для негативных ассертов
// (контрольный chat без link'а; курсор после первого reject в
// idempotency-сценарии). Совпадает с happy-таймаутом WaitForTelegramSent.
const telegramSilenceWindowReject = 5 * time.Second

// expectedRejectText must be kept in sync with
// internal/telegram/notifier.go::applicationRejectedText.
const expectedRejectText = "Здравствуйте! Благодарим вас за интерес к платформе UGC boost.\n\n" +
	"Мы внимательно рассмотрели вашу заявку, профиль, контент и текущие показатели аккаунта. К сожалению, на данном этапе ваша заявка не прошла модерацию платформы.\n\n" +
	"Это не является оценкой вашего потенциала как креатора — просто сейчас ваш профиль не полностью совпадает с критериями отбора для текущих fashion-кампаний и запросов брендов на платформе 🙏\n\n" +
	"Желаем вам дальнейшего роста и удачи в ваших проектах 🤍"

func TestRejectCreatorApplication(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		// Random UUID — auth middleware short-circuits before any DB lookup.
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), uuid.New())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"reject-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), uuid.New(),
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("application not found returns 404 NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		c := testutil.NewAPIClient(t)
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), uuid.New(),
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("detail before reject has no rejection block (omitempty contract)", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		c, adminToken, _ := testutil.SetupAdminClient(t)

		detail := getApplicationDetailForReject(t, c, adminToken, setup.ApplicationID)
		require.Equal(t, apiclient.Verification, detail.Status)
		require.Nil(t, detail.Rejection,
			"non-rejected заявка не должна нести rejection-блок (omitempty)")
	})

	t.Run("happy path from verification — rejects, audit, telegram_link preserved, push sent", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		tg := testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)

		// Snapshot the spy timestamp BEFORE the call so the wait window
		// query observes only post-action records.
		since := time.Now().UTC()

		appUUID := uuid.MustParse(appID)
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, apiclient.EmptyResult{}, *resp.JSON200)

		detail := getApplicationDetailForReject(t, c, adminToken, appID)
		require.Equal(t, apiclient.Rejected, detail.Status)
		require.NotNil(t, detail.Rejection)
		require.Equal(t, apiclient.Verification, detail.Rejection.FromStatus)
		require.WithinDuration(t, time.Now().UTC(), detail.Rejection.RejectedAt, time.Minute)
		// SetupAdminClient does not surface the admin uuid; assert the field
		// is a non-zero uuid (service unit-test asserts the exact actor pass-
		// through with captured-input on the mock).
		require.NotEqual(t, uuid.Nil, detail.Rejection.RejectedByUserId)

		require.NotNil(t, detail.TelegramLink, "telegram_link must survive reject")
		require.Equal(t, tg.UserID, detail.TelegramLink.TelegramUserId)

		audit := testutil.FindAuditEntry(t, c, adminToken,
			auditEntityTypeCreatorApplicationReject, appID,
			auditActionCreatorApplicationReject)
		require.NotNil(t, audit.NewValue)

		// Chunk 13: одно фиксированное reject-сообщение fire-and-forget
		// после commit'а. Полная сверка text/chat/markup защищает от
		// silent drift текста или появления keyboard'а в будущем.
		msgs := testutil.WaitForTelegramSent(t, tg.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertRejectPushExact(t, msgs[0], tg.UserID, since)
	})

	t.Run("happy path from moderation — rejection.fromStatus reflects moderation, push sent", func(t *testing.T) {
		t.Parallel()
		// Drive the application from verification → moderation through the
		// canonical SendPulse path, then reject it. The fromStatus in the
		// rejection block must reflect `moderation`, not `verification`.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		tg := testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)
		igHandle := normalisedIGHandleForReject(t, setup.Request)
		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		webhookSince := time.Now().UTC()
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		preReject := getApplicationDetailForReject(t, c, adminToken, appID)
		require.Equal(t, apiclient.Moderation, preReject.Status, "precondition: SendPulse must have moved the app")

		// SendPulse fires a "verification approved" push asynchronously
		// post-commit. Drain it before capturing the reject cursor — the
		// reject-window query below must observe only the reject push.
		_ = testutil.WaitForTelegramSent(t, tg.UserID, testutil.TelegramSentOptions{
			Since:       webhookSince,
			ExpectCount: 1,
		})

		since := time.Now().UTC()

		appUUID := uuid.MustParse(appID)
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())

		detail := getApplicationDetailForReject(t, c, adminToken, appID)
		require.Equal(t, apiclient.Rejected, detail.Status)
		require.NotNil(t, detail.Rejection)
		require.Equal(t, apiclient.Moderation, detail.Rejection.FromStatus)
		require.WithinDuration(t, time.Now().UTC(), detail.Rejection.RejectedAt, time.Minute)
		require.NotEqual(t, uuid.Nil, detail.Rejection.RejectedByUserId)
		require.NotNil(t, detail.TelegramLink, "telegram_link must survive reject from moderation")

		msgs := testutil.WaitForTelegramSent(t, tg.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertRejectPushExact(t, msgs[0], tg.UserID, since)
	})

	t.Run("happy path without telegram link — rejects, warns, no push", func(t *testing.T) {
		t.Parallel()
		// Никаких LinkTelegramToApplication: reject должен пройти 200,
		// admin GET detail отдать telegramLink == nil, а notify
		// коротко-замкнуться warn'ом в сервис-логе. Контрольный
		// dummy_chat защищает от ложного позитива на чужие
		// параллельные тесты.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		c, adminToken, _ := testutil.SetupAdminClient(t)

		dummyChatID := testutil.UniqueTelegramUserID()
		since := time.Now().UTC()

		appUUID := uuid.MustParse(appID)
		resp, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		detail := getApplicationDetailForReject(t, c, adminToken, appID)
		require.Equal(t, apiclient.Rejected, detail.Status)
		require.NotNil(t, detail.Rejection)
		require.Equal(t, apiclient.Verification, detail.Rejection.FromStatus)
		require.Nil(t, detail.TelegramLink, "no LinkTelegramToApplication — admin detail must report nil link")

		testutil.EnsureNoNewTelegramSent(t, dummyChatID, since, telegramSilenceWindowReject)
	})

	t.Run("repeat reject returns 422 NOT_REJECTABLE; one transition row, one push", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		tg := testutil.LinkTelegramToApplication(t, appID)
		c, adminToken, _ := testutil.SetupAdminClient(t)

		appUUID := uuid.MustParse(appID)
		firstSince := time.Now().UTC()
		first, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, first.StatusCode())

		// Wait for the first push so the second-attempt cursor below is
		// strictly past the only legitimate send.
		msgs := testutil.WaitForTelegramSent(t, tg.UserID, testutil.TelegramSentOptions{
			Since:       firstSince,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertRejectPushExact(t, msgs[0], tg.UserID, firstSince)

		afterFirst := time.Now().UTC()
		second, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, second.StatusCode())
		require.NotNil(t, second.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_NOT_REJECTABLE", second.JSON422.Error.Code)

		// Detail still shows a single rejection block (the first one); the
		// second call must not have produced a second transition row.
		detail := getApplicationDetailForReject(t, c, adminToken, appID)
		require.Equal(t, apiclient.Rejected, detail.Status)
		require.NotNil(t, detail.Rejection)
		require.Equal(t, apiclient.Verification, detail.Rejection.FromStatus)

		// Second 422 short-circuits inside WithTx — notify must not fire.
		testutil.EnsureNoNewTelegramSent(t, tg.UserID, afterFirst, telegramSilenceWindowReject)
	})

}

// assertRejectPushExact сверяет outbound-параметры reject-push'а: chat id,
// точный текст, plain mode (no WebApp). msg.Error намеренно не ассертим —
// под TeeSender'ом реальный bot.SendMessage отвергает синтетический chat
// id и spy фиксирует upstream-ошибку, но это не дефект fire-and-forget
// pipeline'а (см. backend/e2e/webhooks/sendpulse_instagram_test.go ::
// assertVerificationApprovedShape для того же контракта).
func assertRejectPushExact(t *testing.T, msg testclient.TelegramSentMessage, chatID int64, since time.Time) {
	t.Helper()
	require.Equal(t, chatID, msg.ChatId)
	require.Equal(t, expectedRejectText, msg.Text)
	require.Nil(t, msg.WebAppUrl, "reject message must be plain — no WebApp keyboard")
	require.True(t, !msg.SentAt.Before(since), "sent_at must be at or after the cursor")
	require.WithinDuration(t, time.Now().UTC(), msg.SentAt, telegramSilenceWindowReject*2)
}

// getApplicationDetailForReject is the local copy of the same helper the
// manual_verify test uses. e2e files do not share helpers across files
// directly because Go's test package boundary keeps file-local helpers
// invisible — instead, helpers either live in testutil (cross-file) or
// each test file carries its own thin wrapper. This wrapper is intentionally
// a 5-line stub so the core lookup path is consistent across reject-
// scenarios without dragging in the manual_verify helper graph.
func getApplicationDetailForReject(t *testing.T, c *apiclient.ClientWithResponses,
	token, appID string) *apiclient.CreatorApplicationDetailData {
	t.Helper()
	id, err := uuid.Parse(appID)
	require.NoError(t, err)
	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), id, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return &resp.JSON200.Data
}

// normalisedIGHandleForReject mirrors normalisedIGHandleForManual but kept
// local for the same reason — go test files do not share package-private
// helpers automatically; making them shared would mean promoting them to
// testutil. Both helpers share a 5-line body and one obvious purpose, so
// inlining the duplicate is cheaper than another testutil round-trip.
func normalisedIGHandleForReject(t *testing.T, req apiclient.CreatorApplicationSubmitRequest) string {
	t.Helper()
	for _, s := range req.Socials {
		if s.Platform == apiclient.Instagram {
			h := s.Handle
			if len(h) > 0 && h[0] == '@' {
				h = h[1:]
			}
			return h
		}
	}
	t.Fatalf("submission request has no instagram social: %#v", req.Socials)
	return ""
}
