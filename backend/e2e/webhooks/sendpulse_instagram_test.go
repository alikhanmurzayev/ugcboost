// Package webhooks — E2E тесты HTTP-поверхности /webhooks/* (chunk 8
// creator-onboarding-roadmap).
//
// TestSendPulseInstagramWebhook проходит по всем веткам I/O-матрицы
// SendPulse-вебхука. На вход — заявка, поданная через лендинг (статус
// verification, Instagram-социалка). На выходе мы отслеживаем три сигнала:
// HTTP-код самого вебхука (200 для всех успешных и no-op-исходов, 401 для
// неверного bearer'а — анти-fingerprinting), статус заявки в admin-detail
// (verification → moderation на happy path) и audit-row через
// /audit-logs (creator_application_verification_auto). Записи в
// creator_application_status_transitions проверяются юнит-тестами и
// вручную через psql — публичной/админской ручки чтения для них пока нет
// (отдельный будущий чанк, когда появится UI).
//
// Сценарии:
//   - happy path: код, отправленный SendPulse'ом, совпадает с заявкой,
//     IG-handle совпадает; ожидаем 200 {}, статус заявки moderation,
//     audit-row с handle_changed=false;
//   - self-fix: тот же код, но username отличается от сохранённого handle;
//     заявка всё равно переходит, audit-row помечает handle_changed=true;
//   - already verified: повторная доставка того же вебхука после happy
//     path → 200 {}, статус и handle не меняются, новых audit-строк нет;
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
	"testing"

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

	t.Run("happy path verifies application and writes audit row", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)

		status, respBody := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)
		require.JSONEq(t, "{}", string(respBody))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detail := getApplicationDetail(t, adminClient, adminToken, appID)
		require.Equal(t, apiclient.Moderation, detail.Status)
		ig := findIGSocial(t, detail)
		require.True(t, ig.Verified, "IG social must be verified after webhook")
		require.NotNil(t, ig.Method)
		require.Equal(t, apiclient.Auto, *ig.Method)
		require.Equal(t, igHandle, ig.Handle, "happy path must leave the handle untouched")

		audit := testutil.FindAuditEntry(t, adminClient, adminToken,
			auditEntityTypeCreatorApplication, appID,
			auditActionCreatorApplicationVerificationAuto)
		require.Equal(t, false, auditPayloadField(t, audit, "handle_changed"))
		require.Equal(t, "verification", auditPayloadField(t, audit, "from_status"))
		require.Equal(t, "moderation", auditPayloadField(t, audit, "to_status"))
	})

	t.Run("self-fix mismatch overwrites handle and stamps audit flag", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID

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

		audit := testutil.FindAuditEntry(t, adminClient, adminToken,
			auditEntityTypeCreatorApplication, appID,
			auditActionCreatorApplicationVerificationAuto)
		require.Equal(t, true, auditPayloadField(t, audit, "handle_changed"))
		require.Equal(t, "verification", auditPayloadField(t, audit, "from_status"))
		require.Equal(t, "moderation", auditPayloadField(t, audit, "to_status"))
	})

	t.Run("repeat delivery after success is a no-op", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		igHandle := normalisedIGHandle(t, setup.Request)

		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)

		// First delivery — full happy path.
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		// Second delivery — must short-circuit before any state change.
		status2, body2 := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status2)
		require.JSONEq(t, "{}", string(body2))

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

// auditPayloadField extracts a single JSON property from an audit entry's
// new_value blob, fails the test if the field is absent.
func auditPayloadField(t *testing.T, entry *apiclient.AuditLogEntry, field string) any {
	t.Helper()
	require.NotNil(t, entry.NewValue)
	raw, err := json.Marshal(entry.NewValue)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	v, ok := payload[field]
	require.True(t, ok, "audit payload missing field %q: %s", field, raw)
	return v
}
