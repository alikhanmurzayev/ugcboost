package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// SetupCreatorApplicationViaLandingResult bundles the persisted application
// id with the request body that produced it, so callers can both register
// follow-up cleanup and assert on the values they sent without re-derivation.
type SetupCreatorApplicationViaLandingResult struct {
	ApplicationID string
	Request       apiclient.CreatorApplicationSubmitRequest
}

// SetupCreatorApplicationViaLanding submits an application through the public
// landing endpoint (POST /creators/applications) and registers automatic
// cleanup of the resulting row. The optional mutate hooks let callers tweak
// any field of the request before it is sent — list-tests use them to vary
// city/categories/age/social handles so the e2e dataset reflects the
// filter/sort scenarios they exercise.
//
// IINs are generated via UniqueIIN so concurrent test runs do not collide on
// the partial unique index, and the whole helper runs through real business
// flow (no DB seeds) to honour the spec's "test data only via business
// endpoints" rule.
func SetupCreatorApplicationViaLanding(t *testing.T, mutate ...func(*apiclient.CreatorApplicationSubmitRequest)) SetupCreatorApplicationViaLandingResult {
	t.Helper()
	iin := UniqueIIN()
	req := defaultCreatorApplicationRequest(iin)
	for _, m := range mutate {
		m(&req)
	}
	c := NewAPIClient(t)
	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	appID := resp.JSON201.Data.ApplicationId.String()
	RegisterCreatorApplicationCleanup(t, appID)
	return SetupCreatorApplicationViaLandingResult{ApplicationID: appID, Request: req}
}

// GetCreatorApplicationVerificationCode fetches the persisted verification
// code for an application via the test-only endpoint. Used by SendPulse
// webhook tests to construct realistic IG DM payloads — the production API
// hides the column.
func GetCreatorApplicationVerificationCode(t *testing.T, applicationID string) string {
	t.Helper()
	tc := NewTestClient(t)
	id, err := uuid.Parse(applicationID)
	require.NoError(t, err)
	resp, err := tc.GetCreatorApplicationVerificationCodeWithResponse(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.VerificationCode
}

// LinkTelegramToApplication drives /start <applicationID> from a fresh
// synthetic Telegram account and asserts the bot replied successfully. The
// returned TelegramUpdate carries the user_id/username/first/last names the
// helper synthesised so list tests can verify telegramLinked is propagated.
func LinkTelegramToApplication(t *testing.T, applicationID string) TelegramUpdate {
	t.Helper()
	tc := NewTestClient(t)
	upd := DefaultTelegramUpdate(t)
	upd.Text = "/start " + applicationID
	replies := SendTelegramUpdate(t, tc, upd)
	require.Len(t, replies, 1, "telegram bot must reply exactly once to /start <appID>")
	return upd
}

// defaultCreatorApplicationRequest builds the canonical "good" submission for
// the helper to mutate. Per-IIN suffix on social handles guarantees uniqueness
// without leaking PII into static literals.
func defaultCreatorApplicationRequest(iin string) apiclient.CreatorApplicationSubmitRequest {
	middle := "Ивановна"
	suffix := iin[7:]
	return apiclient.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		Iin:        iin,
		Phone:      "+77001234567",
		City:       "almaty",
		Categories: []string{"beauty", "fashion"},
		Socials: []apiclient.SocialAccountInput{
			{Platform: apiclient.Instagram, Handle: "@aidana_" + suffix},
			{Platform: apiclient.Tiktok, Handle: "aidana_tt_" + suffix},
		},
		AcceptedAll: true,
	}
}
