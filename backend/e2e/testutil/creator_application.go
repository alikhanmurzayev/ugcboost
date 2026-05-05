package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// SocialFixture verification kinds. The setup helper accepts at most ONE
// social with Verification != VerificationNone per fixture: API rules say a
// successful manual or SendPulse-driven verification flips the application
// from `verification` to `moderation`, after which any further verification
// fails (NOT_IN_VERIFICATION). The fixture commits to one verified social
// per application, the rest stay unverified.
const (
	VerificationAutoIG = "auto-ig"
	VerificationManual = "manual"
	VerificationNone   = "none"
)

// SocialFixture is one social account attached to a CreatorApplicationFixture.
// Caller-controlled fields drive submit; the helper populates SocialID after
// the application is persisted, and (for verified rows) VerifiedByAdminID +
// VerifiedAt after the corresponding verification call returns.
type SocialFixture struct {
	Platform     string
	Handle       string
	Verification string

	// Populated by SetupCreatorApplicationInModeration.
	SocialID          string
	VerifiedByAdminID *string
	VerifiedAt        *time.Time
}

// CreatorApplicationFixture carries everything the moderation-stage scenarios
// need to drive an application from submit through to `moderation` and assert
// the resulting creator aggregate. Caller fills the upper block; the helper
// fills the lower block (ApplicationID + Telegram + per-social ids/timestamps).
type CreatorApplicationFixture struct {
	// Inputs.
	LastName          string
	FirstName         string
	MiddleName        *string
	IIN               string
	Phone             string
	CityCode          string
	Address           *string
	CategoryCodes     []string
	CategoryOtherText *string
	Socials           []SocialFixture

	// Populated by SetupCreatorApplicationInModeration.
	ApplicationID     string
	AdminToken        string
	BirthDate         time.Time
	TelegramUserID    int64
	TelegramUsername  *string
	TelegramFirstName *string
	TelegramLastName  *string
}

// SetupCreatorApplicationInModeration submits an application via the public
// landing endpoint, drives the /start Telegram link from a synthetic account,
// applies exactly one verification (auto-IG or manual) to lift the status to
// `moderation`, and returns the fully populated fixture so the caller can
// proceed to approve and assert against the resulting creator aggregate.
//
// Constraint: the fixture must declare exactly one social with
// Verification != VerificationNone. Two or more verifications cannot be
// applied through the public API — the second call would observe status
// already moved past `verification` and return NOT_IN_VERIFICATION /
// NotFound. The helper t.Fatals if this constraint is violated.
//
// Cleanup is registered through SetupCreatorApplicationViaLanding; callers
// that produce a creator (POST approve) must additionally call
// RegisterCreatorCleanup AFTER receiving the creator id so LIFO removes the
// creator before the application that owns it.
func SetupCreatorApplicationInModeration(t *testing.T, in CreatorApplicationFixture) CreatorApplicationFixture {
	t.Helper()

	verifiedCount := 0
	for _, s := range in.Socials {
		if s.Verification != VerificationNone {
			verifiedCount++
		}
	}
	if verifiedCount != 1 {
		t.Fatalf("SetupCreatorApplicationInModeration: exactly one social must carry Verification != %q; got %d", VerificationNone, verifiedCount)
	}

	if in.IIN == "" {
		in.IIN = UniqueIIN()
	}
	if in.LastName == "" {
		in.LastName = "Муратова"
	}
	if in.FirstName == "" {
		in.FirstName = "Айдана"
	}
	if in.Phone == "" {
		in.Phone = "+77001234567"
	}
	if in.CityCode == "" {
		in.CityCode = "almaty"
	}
	if len(in.CategoryCodes) == 0 {
		in.CategoryCodes = []string{"beauty"}
	}
	if len(in.Socials) == 0 {
		t.Fatalf("SetupCreatorApplicationInModeration: at least one social required (the verified one)")
	}

	apiSocials := make([]apiclient.SocialAccountInput, len(in.Socials))
	for i, s := range in.Socials {
		apiSocials[i] = apiclient.SocialAccountInput{
			Platform: apiclient.SocialPlatform(s.Platform),
			Handle:   s.Handle,
		}
	}
	req := apiclient.CreatorApplicationSubmitRequest{
		LastName:          in.LastName,
		FirstName:         in.FirstName,
		MiddleName:        in.MiddleName,
		Iin:               in.IIN,
		Phone:             in.Phone,
		City:              in.CityCode,
		Address:           in.Address,
		Categories:        in.CategoryCodes,
		CategoryOtherText: in.CategoryOtherText,
		Socials:           apiSocials,
		AcceptedAll:       true,
	}

	setup := SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
		*r = req
	})
	in.ApplicationID = setup.ApplicationID

	tg := LinkTelegramToApplication(t, in.ApplicationID)
	in.TelegramUserID = tg.UserID
	in.TelegramUsername = tg.Username
	in.TelegramFirstName = tg.FirstName
	in.TelegramLastName = tg.LastName

	c, adminToken, _ := SetupAdminClient(t)
	in.AdminToken = adminToken

	detail := getCreatorApplicationDetailForFixture(t, c, adminToken, in.ApplicationID)
	in.BirthDate = detail.BirthDate.Time

	for i := range in.Socials {
		match := findApplicationSocialByPlatform(detail.Socials, in.Socials[i].Platform)
		if match == nil {
			t.Fatalf("setup fixture: persisted application has no social for platform %q", in.Socials[i].Platform)
		}
		in.Socials[i].SocialID = match.Id.String()
		// The backend lower-cases / strips the leading @ — pull the
		// canonical value back into the fixture so downstream asserts compare
		// against the same string the aggregate will return.
		in.Socials[i].Handle = match.Handle
	}

	for i := range in.Socials {
		switch in.Socials[i].Verification {
		case VerificationAutoIG:
			require.Equalf(t, string(apiclient.Instagram), in.Socials[i].Platform,
				"VerificationAutoIG only fits the Instagram platform; got %q", in.Socials[i].Platform)
			code := GetCreatorApplicationVerificationCode(t, in.ApplicationID)
			webhookSince := time.Now().UTC()
			body := SendPulseWebhookHappyPathRequest(code, in.Socials[i].Handle)
			status, _ := SendPulseWebhookCall(t, SendPulseWebhookOptions{Body: &body})
			require.Equal(t, http.StatusOK, status, "fixture: SendPulse webhook for IG auto-verify must succeed")
			// Drain the post-commit verification-approved push so the caller's
			// own WaitForTelegramSent windows around approve see only the
			// approve push, not the welcome notify from this verification.
			_ = WaitForTelegramSent(t, in.TelegramUserID, TelegramSentOptions{
				Since:       webhookSince,
				ExpectCount: 1,
			})
		case VerificationManual:
			socialUUID, err := uuid.Parse(in.Socials[i].SocialID)
			require.NoError(t, err)
			appUUID := uuid.MustParse(in.ApplicationID)
			resp, err := c.VerifyCreatorApplicationSocialWithResponse(context.Background(),
				appUUID, socialUUID,
				apiclient.VerifyCreatorApplicationSocialJSONRequestBody{},
				WithAuth(adminToken))
			require.NoError(t, err)
			require.Equalf(t, http.StatusOK, resp.StatusCode(),
				"fixture: manual verify for %s/%s must return 200", in.Socials[i].Platform, in.Socials[i].Handle)
		}
	}

	post := getCreatorApplicationDetailForFixture(t, c, adminToken, in.ApplicationID)
	require.Equal(t, apiclient.Moderation, post.Status,
		"fixture: application must reach moderation after the configured verification")

	for i := range in.Socials {
		match := findApplicationSocialByID(post.Socials, in.Socials[i].SocialID)
		require.NotNilf(t, match, "fixture: re-fetched detail missing social %s", in.Socials[i].SocialID)
		in.Socials[i].Handle = match.Handle
		in.Socials[i].VerifiedAt = match.VerifiedAt
		if match.VerifiedByUserId != nil {
			s := match.VerifiedByUserId.String()
			in.Socials[i].VerifiedByAdminID = &s
		}
	}

	return in
}

// getCreatorApplicationDetailForFixture wraps the admin GET aggregate so
// fixture-internal lookups stay private to the helper. The setup public
// surface stays minimal.
func getCreatorApplicationDetailForFixture(t *testing.T, c *apiclient.ClientWithResponses, token, appID string) *apiclient.CreatorApplicationDetailData {
	t.Helper()
	id, err := uuid.Parse(appID)
	require.NoError(t, err)
	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), id, WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return &resp.JSON200.Data
}

func findApplicationSocialByPlatform(socials []apiclient.CreatorApplicationDetailSocial, platform string) *apiclient.CreatorApplicationDetailSocial {
	for i := range socials {
		if string(socials[i].Platform) == platform {
			return &socials[i]
		}
	}
	return nil
}

func findApplicationSocialByID(socials []apiclient.CreatorApplicationDetailSocial, id string) *apiclient.CreatorApplicationDetailSocial {
	for i := range socials {
		if socials[i].Id.String() == id {
			return &socials[i]
		}
	}
	return nil
}

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
// code via the admin detail endpoint. Used by SendPulse webhook tests to
// construct realistic IG DM payloads. Public/landing endpoints intentionally
// hide the field — only admins (and these e2e tests, with admin token) see it.
func GetCreatorApplicationVerificationCode(t *testing.T, applicationID string) string {
	t.Helper()
	c, token, _ := SetupAdminClient(t)
	id, err := uuid.Parse(applicationID)
	require.NoError(t, err)
	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), id, WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.NotEmpty(t, resp.JSON200.Data.VerificationCode, "admin detail must surface verification_code")
	return resp.JSON200.Data.VerificationCode
}

// LinkTelegramToApplication drives /start <applicationID> from a fresh
// synthetic Telegram account, asserts the bot did not produce a synchronous
// reply (chunk 9 — welcome is fire-and-forget through the Notifier and lands
// in /test/telegram/sent, not the per-call SpyOnlySender of /test/telegram/
// message), and blocks until the async welcome lands in the spy. Draining
// the welcome here means downstream tests can capture a "since" right after
// the helper returns and trust that follow-up notifications they assert on
// (e.g. verification-approved) will land in isolation. The returned
// TelegramUpdate carries the user_id/username/first/last names the helper
// synthesised so list tests can verify telegramLinked is propagated.
func LinkTelegramToApplication(t *testing.T, applicationID string) TelegramUpdate {
	t.Helper()
	tc := NewTestClient(t)
	upd := DefaultTelegramUpdate(t)
	upd.Text = "/start " + applicationID
	since := time.Now().UTC()
	replies := SendTelegramUpdate(t, tc, upd)
	require.Empty(t, replies, "telegram bot must not produce a synchronous reply on success-link (welcome is async via Notifier)")
	_ = WaitForTelegramSent(t, upd.UserID, TelegramSentOptions{
		Since:       since,
		ExpectCount: 1,
	})
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
