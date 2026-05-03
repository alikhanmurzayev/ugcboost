package testutil

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// envSendPulseSecret carries the bearer secret e2e tests use when calling
// /webhooks/sendpulse/instagram. The backend reads the same env var via
// SENDPULSE_WEBHOOK_SECRET; if either side is unset the default below
// matches the value baked into backend/.env (local) so out-of-the-box
// `make test-e2e-backend` works against a hand-spun stack.
const envSendPulseSecret = "SENDPULSE_WEBHOOK_SECRET"

func sendPulseSecret() string {
	if v := os.Getenv(envSendPulseSecret); v != "" {
		return v
	}
	return "local-dev-sendpulse-secret"
}

// SendPulseWebhookOptions tweaks the default request without forcing every
// test to spell out the full payload. Zero-value Body sends the canonical
// happy-path payload. OverrideToken non-nil substitutes the bearer (used to
// assert the 401 path); empty string sends no Authorization header at all.
type SendPulseWebhookOptions struct {
	Body          *apiclient.SendPulseInstagramWebhookRequest
	OverrideToken *string
}

// SendPulseWebhookCall posts a SendPulse-style payload through the generated
// client and returns parsed status + raw body bytes. Bearer header is set
// via the editor chain so callers can flip auth modes without touching the
// transport. Caller is expected to require.Equal on the status code.
func SendPulseWebhookCall(t *testing.T, opts SendPulseWebhookOptions) (status int, body []byte) {
	t.Helper()

	var payload apiclient.SendPulseInstagramWebhookRequest
	if opts.Body != nil {
		payload = *opts.Body
	}

	editors := []apiclient.RequestEditorFn{authEditor(opts.OverrideToken)}

	resp, err := NewAPIClient(t).SendPulseInstagramWebhookWithResponse(
		context.Background(), payload, editors...)
	require.NoError(t, err)
	return resp.StatusCode(), resp.Body
}

// authEditor decides which Authorization header (if any) to attach.
// override == nil      → default Bearer <env-or-local-dev-secret>
// override != nil, ""  → no Authorization header at all
// override != nil, "X" → Bearer X
func authEditor(override *string) apiclient.RequestEditorFn {
	switch {
	case override == nil:
		return func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+sendPulseSecret())
			return nil
		}
	case *override == "":
		return func(_ context.Context, req *http.Request) error {
			req.Header.Del("Authorization")
			return nil
		}
	default:
		token := *override
		return func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}
	}
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
