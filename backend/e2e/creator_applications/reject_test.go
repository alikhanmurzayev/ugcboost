// Package creator_applications — E2E HTTP-поверхность
// POST /creators/applications/{id}/reject (chunk 12 onboarding-roadmap'а):
// admin отклоняет заявку из `verification` или `moderation`, заявка
// переходит в терминал `rejected`, в админ-аггрегате появляется блок
// `rejection`, telegram_link остаётся живым (понадобится chunk 14 для
// уведомления) и **никакого** Telegram-пуша от этого чанка не уходит —
// нотификация отправится отдельным сервисом в chunk 14, текст там статичный.
//
// TestRejectCreatorApplication прогоняет всю I/O-матрицу одной функцией:
// сначала authn/authz (401/403), затем 404 на random uuid, затем happy path
// из verification и из moderation (последний достигается через SendPulse-
// webhook → moderation), затем граничные случаи — повторный reject = 422,
// admin GET до reject'а не содержит блока (omitempty-инвариант openapi),
// telegram_link на reject'е сохраняется. Все t.Run параллельны, каждый
// создаёт изолированную заявку через SetupCreatorApplicationViaLanding и
// чистится в LIFO-стеке (E2E_CLEANUP), родительская заявка тащит соцсети,
// audit, transitions и telegram_link каскадом.
package creator_applications_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// auditActionCreatorApplicationReject mirrors the backend constant
// AuditActionCreatorApplicationReject. e2e is its own Go module, so the
// value is re-declared locally instead of imported.
const auditActionCreatorApplicationReject = "creator_application_reject"

// auditEntityTypeCreatorApplicationReject mirrors AuditEntityTypeCreatorApplication.
const auditEntityTypeCreatorApplicationReject = "creator_application"

// telegramSilenceWindowReject — same 5-second contract as manual_verify_test.go:
// after happy reject we wait the worst-case post-commit delay and assert no
// new push records appeared. Chunk 14 will own the actual notification and
// will replace this assertion with a positive expectation against the same
// spy.
const telegramSilenceWindowReject = 5 * time.Second

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

	t.Run("happy path from verification — rejects, audit, telegram_link preserved, no TG push", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		tg := testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)

		// Snapshot the spy timestamp BEFORE the call so the silence window
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

		// telegram_link preserved — chunk 14 needs the chat to send the
		// rejection notification, and any future broadcast flow needs it
		// for outreach.
		require.NotNil(t, detail.TelegramLink, "telegram_link must survive reject")
		require.Equal(t, tg.UserID, detail.TelegramLink.TelegramUserId)

		audit := testutil.FindAuditEntry(t, c, adminToken,
			auditEntityTypeCreatorApplicationReject, appID,
			auditActionCreatorApplicationReject)
		require.NotNil(t, audit.NewValue)

		// Doctrine-bearing assert: chunk 12 не дёргает Telegram-нотификатор.
		// Chunk 14 заменит этот ассерт на позитивное ожидание (одно сообщение
		// со статическим текстом).
		testutil.EnsureNoNewTelegramSent(t, tg.UserID, since, telegramSilenceWindowReject)
	})

	t.Run("happy path from moderation — rejection.fromStatus reflects moderation", func(t *testing.T) {
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
		// post-commit. Drain it before capturing the silence-window cursor
		// — otherwise EnsureNoNewTelegramSent below would observe this
		// pre-existing approval as a chunk-12-induced send.
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

		// SendPulse-path notification fires *one* push (verification approved).
		// Reject itself must not add anything — silence window catches a
		// spurious extra push if chunk 12 ever tries to notify.
		testutil.EnsureNoNewTelegramSent(t, tg.UserID, since, telegramSilenceWindowReject)
	})

	t.Run("repeat reject returns 422 NOT_REJECTABLE; one transition row per application", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		testutil.LinkTelegramToApplication(t, appID)
		c, adminToken, _ := testutil.SetupAdminClient(t)

		appUUID := uuid.MustParse(appID)
		first, err := c.RejectCreatorApplicationWithResponse(context.Background(), appUUID,
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, first.StatusCode())

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
	})

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
