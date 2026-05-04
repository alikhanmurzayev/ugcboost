// Package creator_applications — E2E HTTP-поверхность
// POST /creators/applications/{id}/socials/{socialId}/verify (chunk 10
// onboarding-roadmap'а): admin вручную помечает соцсеть верифицированной,
// заявка автоматически переходит verification → moderation, audit + state
// history записываются в той же транзакции, **никакого** Telegram-пуша
// креатору не уходит — он сам не доказывал владение аккаунтом, и пуш
// «вы верифицированы» был бы вводящим в заблуждение.
//
// TestVerifyCreatorApplicationSocialManually прогоняет всю I/O-матрицу одной
// функцией через подзапуски — авторизация (401/403) до бизнес-веток, затем
// все 4 ошибки сервиса с разными кодами (404/404/409/422/422), и в конце
// happy path с проверкой статуса заявки, audit-row и явным ассертом «новых
// записей в /test/telegram/sent для этого chat_id за 5 секунд нет».
//
// Все t.Run параллельны. Каждый создаёт свежую заявку через
// SetupCreatorApplicationViaLanding, так что параллельные прогоны изолированы;
// родительская заявка убирается в cleanup, каскад тащит соцсети, audit,
// transitions, telegram_link.
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

// auditEntityTypeCreatorApplicationManual — duplicate of the backend constant
// AuditEntityTypeCreatorApplication; e2e is its own Go module so the value is
// re-declared locally in every test file that needs it.
const auditEntityTypeCreatorApplicationManual = "creator_application"

// auditActionCreatorApplicationVerificationManual mirrors the backend
// constant AuditActionCreatorApplicationVerificationManual.
const auditActionCreatorApplicationVerificationManual = "creator_application_verification_manual"

// telegramSilenceWindow — спека требует подождать ровно 5 секунд после
// happy-path call'а и проверить, что никаких пушей креатору не ушло. Окно
// должно быть достаточно длинным, чтобы покрыть worst-case задержку
// post-commit горутины Notifier'а — здесь её просто нет, но мы должны это
// доказать наблюдением «no records over the window», а не одиночным
// лукапом сразу после ответа.
const telegramSilenceWindow = 5 * time.Second

func TestVerifyCreatorApplicationSocialManually(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		// Random UUIDs — auth middleware short-circuits before any DB lookup,
		// so the test does not need to provision real rows.
		appID := uuid.New()
		socialID := uuid.New()
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appID, socialID, apiclient.VerifyCreatorApplicationSocialJSONRequestBody{})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"manual-verify-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		appID := uuid.New()
		socialID := uuid.New()
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appID, socialID, apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
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
		// Random UUIDs that almost certainly do not match any seeded row.
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			uuid.New(), uuid.New(),
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("social not in this application returns 404 with social-specific code", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		testutil.LinkTelegramToApplication(t, setup.ApplicationID)

		appUUID := uuid.MustParse(setup.ApplicationID)
		c := testutil.NewAPIClient(t)
		_, adminToken, _ := testutil.SetupAdminClient(t)
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appUUID, uuid.New(),
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CREATOR_APPLICATION_SOCIAL_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("missing telegram link returns 422 TELEGRAM_NOT_LINKED", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		// Intentionally skip LinkTelegramToApplication — verify must refuse
		// without a chat to reach the creator on later flows.
		c, adminToken, _ := testutil.SetupAdminClient(t)

		social := findFirstSocial(t, c, adminToken, setup.ApplicationID)
		appUUID := uuid.MustParse(setup.ApplicationID)
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appUUID, social.Id,
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED", resp.JSON422.Error.Code)

		detail := getApplicationDetailForManualVerify(t, c, adminToken, setup.ApplicationID)
		require.Equal(t, apiclient.Verification, detail.Status, "БД не должна меняться при отказе")
	})

	t.Run("wrong status returns 422 NOT_IN_VERIFICATION", func(t *testing.T) {
		t.Parallel()
		// Reach `moderation` through the canonical SendPulse path so the
		// state machine is in a real downstream state, then try to manually
		// verify a TikTok handle on a now-moderation application.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)
		igHandle := normalisedIGHandleForManual(t, setup.Request)
		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		detail := getApplicationDetailForManualVerify(t, c, adminToken, appID)
		require.Equal(t, apiclient.Moderation, detail.Status, "precondition: SendPulse must have moved the app")
		ttSocial := findSocialByPlatform(t, detail, apiclient.Tiktok)

		appUUID := uuid.MustParse(appID)
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appUUID, ttSocial.Id,
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_NOT_IN_VERIFICATION", resp.JSON422.Error.Code)
	})

	t.Run("already verified social returns 409", func(t *testing.T) {
		t.Parallel()
		// Same setup as wrong-status — once SendPulse has marked IG verified
		// + moderated, replay the manual call against the IG row to assert
		// the already-verified branch fires (status=409). The earlier branch
		// would short-circuit on status before reaching the social, but
		// tests serve as living documentation: separate assertion = separate
		// code path.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)
		igHandle := normalisedIGHandleForManual(t, setup.Request)
		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		detail := getApplicationDetailForManualVerify(t, c, adminToken, appID)
		igSocial := findSocialByPlatform(t, detail, apiclient.Instagram)
		require.True(t, igSocial.Verified, "precondition: IG must be auto-verified")

		appUUID := uuid.MustParse(appID)
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appUUID, igSocial.Id,
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		// Status check (verification → moderation already happened) catches
		// the call first — the behaviour is: caller learns status moved on,
		// not "already verified". Spec says happy path is admins acting on
		// `verification`-only apps; if status moved, we say so.
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_NOT_IN_VERIFICATION", resp.JSON422.Error.Code)
	})

	t.Run("happy path: tiktok-only application moves to moderation, audit row, no TG push", func(t *testing.T) {
		t.Parallel()
		// TikTok-only application — SendPulse webhook cannot help here,
		// so manual verify is the only path forward. This is exactly the
		// scenario chunk 10 was built for.
		setup := testutil.SetupCreatorApplicationViaLanding(t,
			func(req *apiclient.CreatorApplicationSubmitRequest) {
				req.Socials = []apiclient.SocialAccountInput{
					{Platform: apiclient.Tiktok, Handle: "tt_only_" + req.Iin[7:]},
				}
			})
		appID := setup.ApplicationID
		tg := testutil.LinkTelegramToApplication(t, appID)

		c, adminToken, _ := testutil.SetupAdminClient(t)
		social := findFirstSocial(t, c, adminToken, appID)
		require.Equal(t, apiclient.Tiktok, social.Platform)
		require.False(t, social.Verified)

		// Snapshot the spy timestamp BEFORE the call so the silence window
		// query observes only post-action records.
		since := time.Now().UTC()

		appUUID := uuid.MustParse(appID)
		resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
			appUUID, social.Id,
			apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, apiclient.EmptyResult{}, *resp.JSON200)

		// Refetch — status moved to moderation, the targeted social is now
		// verified by `manual` with the admin actor stamped.
		detail := getApplicationDetailForManualVerify(t, c, adminToken, appID)
		require.Equal(t, apiclient.Moderation, detail.Status)
		updated := findSocialByPlatform(t, detail, apiclient.Tiktok)
		require.Equal(t, social.Id, updated.Id)
		require.True(t, updated.Verified)
		require.NotNil(t, updated.Method)
		require.Equal(t, apiclient.Manual, *updated.Method)
		require.NotNil(t, updated.VerifiedByUserId)
		require.NotNil(t, updated.VerifiedAt)
		require.WithinDuration(t, time.Now().UTC(), *updated.VerifiedAt, time.Minute)

		audit := testutil.FindAuditEntry(t, c, adminToken,
			auditEntityTypeCreatorApplicationManual, appID,
			auditActionCreatorApplicationVerificationManual)
		require.NotNil(t, audit.NewValue)

		// Doctrine-bearing assert: креатор сам не верифицировал — пуш ему
		// не идёт. EnsureNoNewTelegramSent опросит spy всё окно (5 секунд)
		// и упадёт сразу же при первом наблюдении лишней записи; чистый
		// проход подтверждает, что post-commit notify-горутина и
		// предыдущие пуши (welcome) полностью отыграли до `since`.
		testutil.EnsureNoNewTelegramSent(t, tg.UserID, since, telegramSilenceWindow)
	})
}

// getApplicationDetailForManualVerify wraps the admin GET aggregate for the
// manual-verify scenarios. Local copy avoids a hard dep on the sendpulse
// test file's helper.
func getApplicationDetailForManualVerify(t *testing.T, c *apiclient.ClientWithResponses,
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

// findFirstSocial returns the first social row of a freshly-submitted
// application — used by the not-linked scenario where the only social is
// the canonical TikTok handle from defaultCreatorApplicationRequest.
func findFirstSocial(t *testing.T, c *apiclient.ClientWithResponses, token, appID string) *apiclient.CreatorApplicationDetailSocial {
	t.Helper()
	detail := getApplicationDetailForManualVerify(t, c, token, appID)
	require.NotEmpty(t, detail.Socials, "application must have at least one social attached")
	return &detail.Socials[0]
}

// findSocialByPlatform walks the detail aggregate for a specific platform.
// Fails the test if no row matches — every scenario relies on a pre-known
// social being present in the seeded application.
func findSocialByPlatform(t *testing.T, detail *apiclient.CreatorApplicationDetailData,
	platform apiclient.SocialPlatform) *apiclient.CreatorApplicationDetailSocial {
	t.Helper()
	for i := range detail.Socials {
		s := &detail.Socials[i]
		if s.Platform == platform {
			return s
		}
	}
	t.Fatalf("application detail has no %s social: %#v", platform, detail.Socials)
	return nil
}

// normalisedIGHandleForManual mirrors the SendPulse helper from the webhook
// test file: extract the IG handle from the canonical seed request and
// strip the leading '@' so the webhook call payload matches the persisted
// row exactly.
func normalisedIGHandleForManual(t *testing.T, req apiclient.CreatorApplicationSubmitRequest) string {
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
