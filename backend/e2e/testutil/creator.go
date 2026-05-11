package testutil

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// ApprovedCreatorFixture bundles the persisted state of an approved creator —
// the creator id materialised in `creators` after admin-approve, plus the
// originating application snapshot (PII, Telegram block, social handles)
// SetupApprovedCreator copied into it. List-tests assert against this
// fixture's fields directly (search hits by IIN/handle/phone, sort across
// last_name, filter by city/category) without re-deriving them through the
// admin GET aggregate.
type ApprovedCreatorFixture struct {
	CreatorID  string
	AdminToken string

	IIN              string
	LastName         string
	FirstName        string
	MiddleName       *string
	BirthDate        time.Time
	Phone            string
	CityCode         string
	CategoryCodes    []string
	Socials          []SocialFixture
	TelegramUserID   int64
	TelegramUsername *string
}

// SetupApprovedCreator drives an application from submit through to the
// approved-creator state and registers the LIFO cleanup for both rows. The
// flow reuses SetupCreatorApplicationInModeration (which submits, links
// Telegram and runs the configured verification to lift the application into
// `moderation`), then admin-approves through the public POST endpoint so the
// snapshot lands in `creators` exactly the way a production admin click
// would.
//
// Cleanup order matters: the application cleanup is registered first by the
// inner SetupCreatorApplicationViaLanding, the creator cleanup is registered
// here AFTER. t.Cleanup fires LIFO, so the creator goes away first — the FK
// creators.source_application_id has no ON DELETE clause, and the parent
// application cannot be removed while the child still references it.
//
// Pass an opts.Socials slice where exactly one entry carries Verification !=
// VerificationNone (helper-internal constraint, see SetupCreatorApplicationInModeration).
// Caller-controlled IIN/category/city are wired through unchanged so list
// tests can drive the dataset deterministically.
func SetupApprovedCreator(t *testing.T, opts CreatorApplicationFixture) ApprovedCreatorFixture {
	t.Helper()

	app := SetupCreatorApplicationInModeration(t, opts)

	c := NewAPIClient(t)
	appUUID, err := uuid.Parse(app.ApplicationID)
	require.NoError(t, err)
	approveStartedAt := time.Now().UTC().Add(-time.Second)
	approveResp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
		apiclient.CreatorApprovalInput{}, WithAuth(app.AdminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, approveResp.StatusCode(),
		"SetupApprovedCreator: approve must return 200, got %d", approveResp.StatusCode())
	require.NotNil(t, approveResp.JSON200)
	creatorID := approveResp.JSON200.Data.CreatorId
	require.NotEqual(t, uuid.Nil, creatorID, "SetupApprovedCreator: approve must yield a fresh creator id")
	RegisterCreatorCleanup(t, creatorID.String())

	// Mark the synthetic chat as test-fake so chunk-12 outbound notify /
	// remind hits the spy-only path even when the backend runs with a real
	// bot token. SpyOnlySender ignores the registration; TeeSender skips
	// the real call. No-op when the test never sends to this chat.
	RegisterTelegramSpyFakeChat(t, app.TelegramUserID)

	// Drain the async NotifyApplicationApproved fired by ApproveCreatorApplication
	// (fire-and-forget through Notifier — see service/creator_application.go).
	// Without this wait the post-approve SendMessage races with any subsequent
	// test action that touches this chat's spy state — most notably
	// RegisterTelegramSpyFailNext, which gets consumed by the in-flight approve
	// message and silently disarms the partial-success scenario.
	_ = WaitForTelegramSent(t, app.TelegramUserID, TelegramSentOptions{
		Since:       approveStartedAt,
		ExpectCount: 1,
	})

	return ApprovedCreatorFixture{
		CreatorID:        creatorID.String(),
		AdminToken:       app.AdminToken,
		IIN:              app.IIN,
		LastName:         app.LastName,
		FirstName:        app.FirstName,
		MiddleName:       app.MiddleName,
		BirthDate:        app.BirthDate,
		Phone:            app.Phone,
		CityCode:         app.CityCode,
		CategoryCodes:    append([]string(nil), app.CategoryCodes...),
		Socials:          append([]SocialFixture(nil), app.Socials...),
		TelegramUserID:   app.TelegramUserID,
		TelegramUsername: app.TelegramUsername,
	}
}

// RegisterCreatorCleanup schedules a POST /test/cleanup-entity for a creator
// row after the test. The testapi handler delegates to CreatorRepo.DeleteForTests
// which cascades through creator_socials and creator_categories. 404 is treated
// as success — the row may have been removed by another step. Cleanups are
// stacked LIFO via t.Cleanup so registering this AFTER the parent application
// cleanup ensures the creator goes first; the FK creators.source_application_id
// has no ON DELETE clause, so the application cannot be deleted while a creator
// still references it.
func RegisterCreatorCleanup(t *testing.T, creatorID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.Creator,
			Id:   creatorID,
		})
		if err != nil {
			return fmt.Errorf("cleanup creator %s: %w", creatorID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup creator %s: unexpected status %d", creatorID, resp.StatusCode())
		}
		return nil
	})
}

// DeleteCreatorForTests calls POST /test/cleanup-entity once and asserts the
// row was actually present. Use this when a test wants to prove a creator-row
// was created (a 200/204 success means the testapi found and deleted exactly
// one row, a 404 surfaces as a require-failure here). For ordinary cleanup
// stack registration prefer RegisterCreatorCleanup, which silently treats
// 404 as success.
func DeleteCreatorForTests(t *testing.T, creatorID string) {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.CleanupEntityWithResponse(context.Background(), testclient.CleanupEntityJSONRequestBody{
		Type: testclient.Creator,
		Id:   creatorID,
	})
	if err != nil {
		t.Fatalf("delete creator %s: %v", creatorID, err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		t.Fatalf("delete creator %s: unexpected status %d", creatorID, resp.StatusCode())
	}
}

// AssertCreatorAggregateMatchesSetup compares a CreatorAggregate returned by
// GET /creators/{id} against the fixture used to create it. Two-stage check:
// first dynamic fields (uuids, timestamps) get a WithinDuration / shape
// validation, then a substituted "expected" struct is compared to the
// observed aggregate via require.Equal — so any unexpected difference (a
// field that drifted between fixture and persistence) surfaces immediately
// rather than being hidden behind per-field checks.
//
// Sorted invariants mirror the repo guarantee from chunk 18a: socials
// returned by ListByCreatorIDs are already (platform, handle)-ascending and
// categories are category_code-ascending. The helper builds the expected
// slices in the same order so the structural compare stays meaningful.
func AssertCreatorAggregateMatchesSetup(t *testing.T, fx CreatorApplicationFixture, creatorID string, aggregate apiclient.CreatorAggregate) {
	t.Helper()

	creatorUUID, err := uuid.Parse(creatorID)
	require.NoError(t, err, "AssertCreatorAggregateMatchesSetup: creatorID must be a valid UUID")
	sourceAppUUID, err := uuid.Parse(fx.ApplicationID)
	require.NoError(t, err, "AssertCreatorAggregateMatchesSetup: fixture ApplicationID must be a valid UUID")

	require.Equal(t, creatorUUID, aggregate.Id, "aggregate.Id")
	require.Equal(t, sourceAppUUID, aggregate.SourceApplicationId, "aggregate.SourceApplicationId")

	now := time.Now().UTC()
	const recentWindow = 5 * time.Minute
	require.WithinDuration(t, now, aggregate.CreatedAt, recentWindow, "aggregate.CreatedAt")
	require.WithinDuration(t, now, aggregate.UpdatedAt, recentWindow, "aggregate.UpdatedAt")
	require.NotEmpty(t, aggregate.CityName, "aggregate.CityName must be hydrated (or fall back to code)")

	sortedSocials := append([]SocialFixture{}, fx.Socials...)
	sort.Slice(sortedSocials, func(i, j int) bool {
		if sortedSocials[i].Platform != sortedSocials[j].Platform {
			return sortedSocials[i].Platform < sortedSocials[j].Platform
		}
		return sortedSocials[i].Handle < sortedSocials[j].Handle
	})
	require.Lenf(t, aggregate.Socials, len(sortedSocials), "aggregate.Socials length")

	expectedSocials := make([]apiclient.CreatorAggregateSocial, len(sortedSocials))
	for i, exp := range sortedSocials {
		actual := aggregate.Socials[i]
		require.Equalf(t, exp.Platform, string(actual.Platform), "social[%d].Platform", i)
		require.Equalf(t, exp.Handle, actual.Handle, "social[%d].Handle", i)
		// social.Id is db-generated by InsertMany at approve time and lives in
		// creator_socials, distinct from the source creator_application_socials
		// id captured in the fixture. Validate it is a real (non-zero) uuid
		// and substitute it back into the expected struct so structural
		// equality survives.
		require.NotEqualf(t, uuid.Nil, actual.Id, "social[%d].Id must be a real uuid", i)
		require.WithinDurationf(t, now, actual.CreatedAt, recentWindow, "social[%d].CreatedAt", i)

		expected := apiclient.CreatorAggregateSocial{
			Id:        actual.Id,
			Platform:  actual.Platform,
			Handle:    exp.Handle,
			Verified:  exp.Verification != VerificationNone,
			CreatedAt: actual.CreatedAt,
		}
		switch exp.Verification {
		case VerificationAutoIG:
			require.NotNilf(t, actual.VerifiedAt, "social[%d].VerifiedAt must be set for auto-verify", i)
			require.WithinDurationf(t, now, *actual.VerifiedAt, recentWindow, "social[%d].VerifiedAt", i)
			method := apiclient.Auto
			expected.Method = &method
			expected.VerifiedByUserId = nil
			expected.VerifiedAt = actual.VerifiedAt
		case VerificationManual:
			require.NotNilf(t, actual.VerifiedAt, "social[%d].VerifiedAt must be set for manual verify", i)
			require.WithinDurationf(t, now, *actual.VerifiedAt, recentWindow, "social[%d].VerifiedAt", i)
			require.NotNilf(t, actual.VerifiedByUserId, "social[%d].VerifiedByUserId must be set for manual verify", i)
			require.NotNilf(t, exp.VerifiedByAdminID, "fixture must capture VerifiedByAdminID after manual verify (social[%d])", i)
			expectedAdminUUID, err := uuid.Parse(*exp.VerifiedByAdminID)
			require.NoError(t, err)
			require.Equalf(t, expectedAdminUUID, *actual.VerifiedByUserId, "social[%d].VerifiedByUserId", i)
			method := apiclient.Manual
			expected.Method = &method
			expected.VerifiedByUserId = actual.VerifiedByUserId
			expected.VerifiedAt = actual.VerifiedAt
		case VerificationNone:
			expected.Method = nil
			expected.VerifiedByUserId = nil
			expected.VerifiedAt = nil
		}
		expectedSocials[i] = expected
	}

	expectedCategoryCodes := append([]string{}, fx.CategoryCodes...)
	sort.Strings(expectedCategoryCodes)
	require.Lenf(t, aggregate.Categories, len(expectedCategoryCodes), "aggregate.Categories length")
	expectedCategories := make([]apiclient.CreatorAggregateCategory, len(expectedCategoryCodes))
	for i, code := range expectedCategoryCodes {
		require.Equalf(t, code, aggregate.Categories[i].Code, "category[%d].Code", i)
		require.NotEmptyf(t, aggregate.Categories[i].Name, "category[%d].Name must be hydrated (or fall back to code)", i)
		expectedCategories[i] = apiclient.CreatorAggregateCategory{
			Code: code,
			Name: aggregate.Categories[i].Name,
		}
	}

	require.Emptyf(t, aggregate.Campaigns,
		"AssertCreatorAggregateMatchesSetup: helper applies to freshly approved creators without campaign attachments; aggregate.Campaigns must be empty")

	expected := apiclient.CreatorAggregate{
		Id:                  aggregate.Id,
		Iin:                 fx.IIN,
		SourceApplicationId: aggregate.SourceApplicationId,
		LastName:            fx.LastName,
		FirstName:           fx.FirstName,
		MiddleName:          fx.MiddleName,
		BirthDate:           openapi_types.Date{Time: fx.BirthDate},
		Phone:               fx.Phone,
		CityCode:            fx.CityCode,
		CityName:            aggregate.CityName,
		Address:             fx.Address,
		CategoryOtherText:   fx.CategoryOtherText,
		TelegramUserId:      fx.TelegramUserID,
		TelegramUsername:    fx.TelegramUsername,
		TelegramFirstName:   fx.TelegramFirstName,
		TelegramLastName:    fx.TelegramLastName,
		Socials:             expectedSocials,
		Categories:          expectedCategories,
		Campaigns:           aggregate.Campaigns,
		CreatedAt:           aggregate.CreatedAt,
		UpdatedAt:           aggregate.UpdatedAt,
	}

	require.Equal(t, expected, aggregate)
}
