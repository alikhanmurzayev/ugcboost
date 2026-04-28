package testutil

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

var (
	counter uint64
	runID   = newRunID()
)

// newRunID builds a per-process identifier for the current test run. Each
// e2e package compiles into its own binary, so when `go test ./...` fans out
// in parallel several runIDs are computed in the same second; the 64 random
// bits from crypto/rand make collisions on the test fixtures (emails, IINs)
// astronomically unlikely. The Unix timestamp prefix stays for debugging —
// it's nice to be able to tell at a glance when a particular run started.
func newRunID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// crypto/rand.Read is documented as never failing on the platforms we
		// support. If the OS entropy source ever does fail, abort the run
		// rather than silently fall back to a weaker source — the whole point
		// of the migration off math/rand is to stop colliding under load.
		panic(fmt.Sprintf("crypto/rand.Read: %v", err))
	}
	return fmt.Sprintf("%d%s", time.Now().Unix(), hex.EncodeToString(b[:]))
}

// UniqueEmail generates a unique email for test isolation.
func UniqueEmail(prefix string) string {
	n := atomic.AddUint64(&counter, 1)
	return fmt.Sprintf("test-%s-%s-%d@e2e.test", prefix, runID, n)
}

// seedUser is the full-detail workhorse behind SeedUser and the Setup*
// helpers. Returns the user ID so callers can register targeted cleanup.
func seedUser(t *testing.T, role string) (id, email, password string) {
	t.Helper()
	tc := NewTestClient(t)
	email = UniqueEmail(role)
	password = DefaultPassword

	resp, err := tc.SeedUserWithResponse(context.Background(), testclient.SeedUserJSONRequestBody{
		Email:    openapi_types.Email(email),
		Password: password,
		Role:     testclient.SeedUserRequestRole(role),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id, email, password
}

// SeedUser creates a user via POST /test/seed-user and returns email + password.
// Prefer SetupAdmin / SetupManager / SetupManagerWithLogin in new tests — they
// also register cleanup. This helper remains for low-level flows that don't
// need auto-cleanup (e.g. password-reset tests that handle teardown manually).
func SeedUser(t *testing.T, role string) (email, password string) {
	_, email, password = seedUser(t, role)
	return email, password
}

// GetResetToken retrieves the raw reset token via GET /test/reset-tokens?email=...
func GetResetToken(t *testing.T, email string) string {
	t.Helper()
	tc := NewTestClient(t)

	resp, err := tc.GetResetTokenWithResponse(context.Background(), &testclient.GetResetTokenParams{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Token
}

// LoginAs logs in and returns the access token. Also populates the cookie jar with refresh token.
func LoginAs(t *testing.T, c *apiclient.ClientWithResponses, email, password string) string {
	t.Helper()
	resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
		Email:    openapi_types.Email(email),
		Password: password,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.AccessToken
}

// SetupAdmin seeds a fresh admin and returns the access token. The seeded
// user is automatically removed after the test via POST /test/cleanup-entity.
func SetupAdmin(t *testing.T) string {
	_, token, _ := SetupAdminClient(t)
	return token
}

// SetupAdminClient seeds a fresh admin, logs in through a new cookie-backed
// API client, and returns (client, accessToken, email). The seeded user is
// automatically removed after the test.
func SetupAdminClient(t *testing.T) (*apiclient.ClientWithResponses, string, string) {
	t.Helper()
	id, email, password := seedUser(t, "admin")
	RegisterUserCleanup(t, id)
	c := NewAPIClient(t)
	token := LoginAs(t, c, email, password)
	return c, token, email
}

// SetupBrand creates a brand through POST /brands using adminToken and
// returns brandID. The brand is automatically removed after the test via
// DELETE /brands/{id} — this reuses the same business endpoint a human
// admin would call, so cleanup goes through the normal audit path.
func SetupBrand(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name string) string {
	t.Helper()
	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: name,
	}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	brandID := resp.JSON201.Data.Id

	RegisterBrandCleanup(t, c, adminToken, brandID)
	return brandID
}

// SetupManager assigns a freshly-generated brand_manager email to the given
// brand via POST /brands/{id}/managers and returns (email, tempPassword).
// The created user is automatically removed after the test. The assignment
// itself is removed as part of the brand cleanup (on brand delete) or as a
// side effect of the user cleanup (audit + brand_managers wiped there).
func SetupManager(t *testing.T, c *apiclient.ClientWithResponses, adminToken, brandID string) (email, password string) {
	t.Helper()
	email = UniqueEmail("mgr")
	resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: email,
	}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	require.NotNil(t, resp.JSON201.Data.TempPassword, "new manager must return a temp password")
	RegisterUserCleanup(t, resp.JSON201.Data.UserId)
	return email, *resp.JSON201.Data.TempPassword
}

// SetupManagerWithLogin combines SetupManager with a login step, returning
// an authenticated client, access token, and the manager's email. The
// manager user is auto-cleaned after the test.
func SetupManagerWithLogin(t *testing.T, adminClient *apiclient.ClientWithResponses, adminToken, brandID string) (*apiclient.ClientWithResponses, string, string) {
	t.Helper()
	email, password := SetupManager(t, adminClient, adminToken, brandID)
	mgrClient := NewAPIClient(t)
	token := LoginAs(t, mgrClient, email, password)
	return mgrClient, token, email
}

// RegisterUserCleanup schedules a POST /test/cleanup-entity for the given
// user after the test. Not found is tolerated — the user may have already
// been removed by a nested cleanup or by a business flow inside the test.
// Exposed so tests that learn a user ID from an API response (not through
// Setup*) can still benefit from the cleanup stack.
func RegisterUserCleanup(t *testing.T, userID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.User,
			Id:   userID,
		})
		if err != nil {
			return fmt.Errorf("cleanup user %s: %w", userID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup user %s: unexpected status %d", userID, resp.StatusCode())
		}
		return nil
	})
}

// RegisterBrandCleanup schedules DELETE /brands/{id} after the test using
// the admin token that created it. Delete goes through the business path on
// purpose — we want audit history for the cleanup too, consistent with how
// a real admin would remove a brand. Exposed for tests that bypass
// SetupBrand to exercise the raw create/delete response shape but still
// need cleanup.
func RegisterBrandCleanup(t *testing.T, c *apiclient.ClientWithResponses, adminToken, brandID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		resp, err := c.DeleteBrandWithResponse(ctx, brandID, WithAuth(adminToken))
		if err != nil {
			return fmt.Errorf("cleanup brand %s: %w", brandID, err)
		}
		if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup brand %s: unexpected status %d", brandID, resp.StatusCode())
		}
		return nil
	})
}
