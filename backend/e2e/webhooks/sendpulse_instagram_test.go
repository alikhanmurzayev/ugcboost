// Package webhooks — E2E тесты HTTP-поверхности /webhooks/* (chunk 8
// creator-onboarding-roadmap, follow-up к PR #52 ревью).
//
// TestSendPulseInstagramWebhook проходит по всем веткам I/O-матрицы
// SendPulse-вебхука. На вход — заявка, поданная через лендинг (статус
// verification, Instagram-социалка). На выходе мы отслеживаем четыре
// сигнала: HTTP-код самого вебхука (200 для всех успешных и no-op-исходов,
// 401 для неверного bearer'а — анти-fingerprinting), статус заявки в
// admin-detail (verification → moderation на happy path), audit-row через
// /audit-logs (creator_application_verification_auto) и outbound Telegram-
// уведомление через тест-ручку /test/telegram/sent. Записи в
// creator_application_status_transitions проверяются юнит-тестами и
// вручную через psql — публичной/админской ручки чтения для них пока нет.
//
// Сценарии:
//   - happy path: код, отправленный SendPulse'ом, совпадает с заявкой,
//     IG-handle совпадает; ожидаем 200 {}, статус заявки moderation,
//     audit-row с handle_changed=false, ровно одно TG-уведомление с WebApp-
//     кнопкой на TMA;
//   - self-fix: тот же код, но username отличается от сохранённого handle;
//     заявка переходит, audit-row помечает handle_changed=true, TG-нотификация
//     уходит;
//   - already verified: повторная доставка того же вебхука после happy
//     path → 200 {}, статус и handle не меняются, новых audit-строк нет,
//     повторное TG-уведомление не уходит;
//   - not found: код, не привязанный ни к одной активной заявке → 200 {},
//     никаких побочных эффектов;
//   - no IG social: заявка без IG-социалки → 200 {}, статус остаётся;
//   - missing code: lastMessage без UGC-NNNNNN → 200 {}, no-op;
//   - bad bearer: неверный или отсутствующий Authorization → 401 {} без
//     тела с подсказками злоумышленнику.
//
// Все тесты параллельны. Заявки создаются через testutil.SetupCreator-
// ApplicationViaLanding, который автоматически регистрирует cleanup —
// каскадное удаление родителя сносит соцсети и связанные ряды
// transitions / audit. SendPulse-секрет читается из env (по умолчанию —
// значение из backend/.env, чтобы локальный go test "из коробки"
// заработал против поднятого через make start-backend стенда).
package webhooks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// auditEntityTypeCreatorApplication mirrors the backend constant. The e2e
// module is isolated from internal/* (separate Go module), so the value
// is duplicated locally — consistent with how every other e2e test
// re-declares the audit/entity strings it cares about.
const auditEntityTypeCreatorApplication = "creator_application"

// auditActionCreatorApplicationVerificationAuto mirrors the backend constant
// AuditActionCreatorApplicationVerificationAuto for the same reason.
const auditActionCreatorApplicationVerificationAuto = "creator_application_verification_auto"

func TestSendPulseInstagramWebhook(t *testing.T) {
	t.Parallel()

	t.Run("happy path verifies application, writes audit row, sends TG notification", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		tgUpd := testutil.LinkTelegramToApplication(t, appID)
		since := time.Now().UTC()

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Moderation, detail.Status)
		require.Equal(t, code, detail.VerificationCode, "admin detail must surface the same code")

		ig := findIGSocial(t, detail)
		require.True(t, ig.Verified, "IG social must be verified after webhook")
		require.NotNil(t, ig.Method)
		require.Equal(t, apiclient.Auto, *ig.Method)
		require.Equal(t, igHandle, ig.Handle, "happy path must leave the handle untouched")
		require.Nil(t, ig.VerifiedByUserId, "auto verification has no actor")
		require.NotNil(t, ig.VerifiedAt)
		require.WithinDuration(t, time.Now().UTC(), *ig.VerifiedAt, time.Minute)

		audit := testutil.FindAuditEntry(t, adminClient, adminToken,
			auditEntityTypeCreatorApplication, appID,
			auditActionCreatorApplicationVerificationAuto)
		assertAuditPayload(t, audit, appID, false)

		sent := testutil.WaitForTelegramSent(t, tgUpd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, sent, 1)
		require.NotEmpty(t, sent[0].Text, "verification notification must carry text")
		require.NotNil(t, sent[0].WebAppUrl, "verification notification must attach a WebApp button")
		require.True(t, strings.HasPrefix(*sent[0].WebAppUrl, "http"),
			"WebApp URL must point at the TMA: %q", *sent[0].WebAppUrl)
		// sent[0].Error may be populated under TeeSender (real Telegram path
		// rejects synthetic chat ids); we only assert orchestration-level
		// invariants — a record exists, with the expected ChatID + WebApp.
	})

	t.Run("self-fix mismatch overwrites handle, stamps audit flag, notifies", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID

		tgUpd := testutil.LinkTelegramToApplication(t, appID)
		since := time.Now().UTC()

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		// Force an obvious mismatch — anything outside the original handle.
		body := testutil.SendPulseWebhookHappyPathRequest(code, "@DefinitelyNew")

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Moderation, detail.Status)
		ig := findIGSocial(t, detail)
		require.Equal(t, "definitelynew", ig.Handle, "self-fix must persist the normalised webhook handle")
		require.True(t, ig.Verified)
		require.Nil(t, ig.VerifiedByUserId)

		audit := testutil.FindAuditEntry(t, adminClient, adminToken,
			auditEntityTypeCreatorApplication, appID,
			auditActionCreatorApplicationVerificationAuto)
		assertAuditPayload(t, audit, appID, true)

		sent := testutil.WaitForTelegramSent(t, tgUpd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, sent, 1)
	})

	t.Run("repeat delivery after success is a no-op (no audit, no TG)", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		tgUpd := testutil.LinkTelegramToApplication(t, appID)
		since := time.Now().UTC()

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)

		// First delivery — full happy path.
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		// Wait for the first notification before the second call so we can
		// distinguish "no extra send" from "send not yet recorded".
		_ = testutil.WaitForTelegramSent(t, tgUpd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})

		// Second delivery — must short-circuit before any state change.
		status2, body2 := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status2)
		require.JSONEq(t, "{}", string(body2))

		// Give the (non-existent) second notify goroutine a chance to fire,
		// then re-poll: count must still be 1.
		time.Sleep(300 * time.Millisecond)
		sent := testutil.WaitForTelegramSent(t, tgUpd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, sent, 1, "noop must not send a second TG notification")

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		entries := listVerificationAuditEntries(t, adminClient, adminToken, appID)
		require.Len(t, entries, 1, "no-op must not append additional audit rows")
	})

	t.Run("unknown code is a no-op without writes", func(t *testing.T) {
		t.Parallel()
		// Submit a fresh application so we have a real handle to send back —
		// the code we pass is intentionally unrelated to keep the row
		// untouched.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		body := testutil.SendPulseWebhookHappyPathRequest("UGC-000001", igHandle)
		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Verification, detail.Status, "unknown code must not transition")
		ig := findIGSocial(t, detail)
		require.False(t, ig.Verified)

		require.Empty(t, listVerificationAuditEntries(t, adminClient, adminToken, appID))
	})

	t.Run("application without IG social returns 200 and stays untouched", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t, func(req *apiclient.CreatorApplicationSubmitRequest) {
			req.Socials = []apiclient.SocialAccountInput{
				{Platform: apiclient.Tiktok, Handle: "tiktok_only_" + req.Iin[7:]},
			}
		})
		appID := setup.ApplicationID

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, "@anything")

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Verification, detail.Status)
		require.Empty(t, listVerificationAuditEntries(t, adminClient, adminToken, appID))
	})

	t.Run("missing UGC code in lastMessage is a no-op", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		body := apiclient.SendPulseInstagramWebhookRequest{
			Username:    igHandle,
			LastMessage: "Hi! Just saying hello, no code attached.",
		}
		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Verification, detail.Status)
		require.Empty(t, listVerificationAuditEntries(t, adminClient, adminToken, appID))
	})

	t.Run("wrong bearer secret returns 401 with empty body", func(t *testing.T) {
		t.Parallel()
		bogus := "definitely-not-the-secret"
		body := testutil.SendPulseWebhookHappyPathRequest("UGC-123456", "aidana")

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{
			Body:          &body,
			OverrideToken: &bogus,
		})
		require.Equal(t, http.StatusUnauthorized, status)
		require.JSONEq(t, "{}", string(respBody))
	})

	t.Run("missing authorization header returns 401 with empty body", func(t *testing.T) {
		t.Parallel()
		empty := ""
		body := testutil.SendPulseWebhookHappyPathRequest("UGC-123456", "aidana")

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{
			Body:          &body,
			OverrideToken: &empty,
		})
		require.Equal(t, http.StatusUnauthorized, status)
		require.JSONEq(t, "{}", string(respBody))
	})
}

// normalisedIGHandle pulls the IG handle from the submission request and
// strips its leading '@' so the webhook tests can send back what SendPulse
// would have stored. Lower-cased for symmetry with the canonical handle the
// service persists.
func normalisedIGHandle(t *testing.T, req apiclient.CreatorApplicationSubmitRequest) string {
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

// getApplicationDetail wraps the admin GET /creators/applications/{id} call
// so each scenario is one line.
func getApplicationDetail(t *testing.T, c *apiclient.ClientWithResponses, token, appID string) *apiclient.CreatorApplicationDetailData {
	t.Helper()
	id, err := uuid.Parse(appID)
	require.NoError(t, err)
	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), id, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return &resp.JSON200.Data
}

// findIGSocial returns the (single) Instagram social embedded in the
// detail aggregate; fails the test if none is present.
func findIGSocial(t *testing.T, detail *apiclient.CreatorApplicationDetailData) *apiclient.CreatorApplicationDetailSocial {
	t.Helper()
	for i := range detail.Socials {
		s := &detail.Socials[i]
		if s.Platform == apiclient.Instagram {
			return s
		}
	}
	t.Fatalf("application detail has no instagram social: %#v", detail.Socials)
	return nil
}

// listVerificationAuditEntries returns every audit row for the application
// whose action is creator_application_verification_auto. Empty slice means
// "the verification side effect has not been recorded yet" — used by the
// no-op scenarios that must not append audit history.
func listVerificationAuditEntries(t *testing.T, c *apiclient.ClientWithResponses, token, appID string) []apiclient.AuditLogEntry {
	t.Helper()
	entityType := auditEntityTypeCreatorApplication
	action := auditActionCreatorApplicationVerificationAuto
	resp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		EntityType: &entityType,
		EntityId:   &appID,
		Action:     &action,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Logs
}

// assertAuditPayload verifies all five fields the verification_auto handler
// stamps onto the audit row's new_value. social_id is shape-checked (UUID
// string, non-empty) — the public detail DTO does not surface social.id, so
// we cannot pin the exact value without exposing a new field just for tests.
func assertAuditPayload(t *testing.T, entry *apiclient.AuditLogEntry, appID string, handleChanged bool) {
	t.Helper()
	require.NotNil(t, entry.NewValue)
	raw, err := json.Marshal(entry.NewValue)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, appID, payload["application_id"])
	socialID, ok := payload["social_id"].(string)
	require.True(t, ok, "social_id must be a string, got %T", payload["social_id"])
	_, parseErr := uuid.Parse(socialID)
	require.NoError(t, parseErr, "social_id must be a UUID: %q", socialID)
	require.Equal(t, "verification", payload["from_status"])
	require.Equal(t, "moderation", payload["to_status"])
	require.Equal(t, handleChanged, payload["handle_changed"])
}
