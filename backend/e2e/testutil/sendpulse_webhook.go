package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// envSendPulseSecret carries the bearer secret e2e tests use when calling
// /webhooks/sendpulse/instagram. The backend reads the same env var via
// SENDPULSE_WEBHOOK_SECRET; if either side is unset the default below
// matches the value baked into backend/.env (local) and backend/.env.ci
// so out-of-the-box `make test-e2e-backend` works in either context.
const envSendPulseSecret = "SENDPULSE_WEBHOOK_SECRET"

// sendPulseWebhookPath is the canonical path the SendPulse middleware gates.
// Held in test-land so we never accidentally re-derive it from the SDK.
const sendPulseWebhookPath = "/webhooks/sendpulse/instagram"

// sendPulseSecret resolves the bearer secret in priority order: explicit
// env var → known dev default. The .env files for local/ci both set this
// env var; falling back is for ad-hoc shell sessions where the operator
// pointed E2E_BASE_URL at a backend they spun up by hand.
func sendPulseSecret() string {
	if v := os.Getenv(envSendPulseSecret); v != "" {
		return v
	}
	return "local-dev-sendpulse-secret"
}

// SendPulseWebhookOptions tweaks one or two knobs of the default request
// without forcing every test to spell out the full payload. Zero-value Body
// (nil) sends the canonical happy-path payload. OverrideToken non-nil
// substitutes the bearer (used to assert the 401 path) — empty string sends
// the request without an Authorization header at all.
type SendPulseWebhookOptions struct {
	Body          *apiclient.SendPulseInstagramWebhookRequest
	OverrideToken *string
}

// SendPulseWebhookCall posts the SendPulse-style payload to the webhook and
// returns the raw HTTP status + body bytes. The caller is expected to
// require.Equal on the status code — every non-network response is a real
// behaviour signal worth asserting against.
//
// Uses the standard retry-enabled HTTPClient so transient transport errors
// are not flaky, but never retries on application 4xx/5xx (per checkRetry).
func SendPulseWebhookCall(t *testing.T, opts SendPulseWebhookOptions) (status int, body []byte) {
	t.Helper()

	var payload apiclient.SendPulseInstagramWebhookRequest
	if opts.Body != nil {
		payload = *opts.Body
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		BaseURL+sendPulseWebhookPath,
		bytes.NewReader(raw),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	switch {
	case opts.OverrideToken == nil:
		req.Header.Set("Authorization", "Bearer "+sendPulseSecret())
	case *opts.OverrideToken == "":
		// no Authorization header — exercises the missing-header branch
	default:
		req.Header.Set("Authorization", "Bearer "+*opts.OverrideToken)
	}

	client := HTTPClient(nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// SendPulseWebhookHappyPathRequest builds the canonical "verified" payload
// for an application that owns the given verification code and IG handle.
// Tests mutate the returned struct in-place when they need a different
// `lastMessage` shape.
func SendPulseWebhookHappyPathRequest(verificationCode, igHandle string) apiclient.SendPulseInstagramWebhookRequest {
	contactID := fmt.Sprintf("contact-%s", verificationCode)
	return apiclient.SendPulseInstagramWebhookRequest{
		Username:    igHandle,
		LastMessage: "Hi UGCBoost! My code is " + verificationCode,
		ContactId:   &contactID,
	}
}
