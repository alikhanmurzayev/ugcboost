package e2etest

// helpers_test.go contains shared test utilities, API client setup, and seed helpers for E2E tests.

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync/atomic"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/e2etest/apiclient"
	"github.com/alikhanmurzayev/ugcboost/e2etest/testclient"
)

var baseURL = getBaseURL()

func getBaseURL() string {
	if url := os.Getenv("E2E_BASE_URL"); url != "" {
		return url
	}
	return "http://localhost:8082"
}

// cfAccessTransport injects Cloudflare Access headers when CF_ACCESS_CLIENT_ID is set.
type cfAccessTransport struct {
	base http.RoundTripper
}

func (t *cfAccessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if id := os.Getenv("CF_ACCESS_CLIENT_ID"); id != "" {
		req.Header.Set("CF-Access-Client-Id", id)
		req.Header.Set("CF-Access-Client-Secret", os.Getenv("CF_ACCESS_CLIENT_SECRET"))
	}
	return t.base.RoundTrip(req)
}

func httpClient(jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:       jar,
		Transport: &cfAccessTransport{base: http.DefaultTransport},
	}
}

var (
	counter uint64
	runID   = fmt.Sprintf("%d%04d", time.Now().Unix(), rand.Intn(10000))
)

func uniqueEmail(prefix string) string {
	n := atomic.AddUint64(&counter, 1)
	return fmt.Sprintf("test-%s-%s-%d@e2e.test", prefix, runID, n)
}

// newAPIClient creates an API client with cookie jar for refresh token support.
func newAPIClient(t *testing.T) *apiclient.ClientWithResponses {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c, err := apiclient.NewClientWithResponses(baseURL,
		apiclient.WithHTTPClient(httpClient(jar)))
	require.NoError(t, err)
	return c
}

// newTestClient creates a client for test-only endpoints.
func newTestClient(t *testing.T) *testclient.ClientWithResponses {
	t.Helper()
	c, err := testclient.NewClientWithResponses(baseURL,
		testclient.WithHTTPClient(httpClient(nil)))
	require.NoError(t, err)
	return c
}

// seedUser creates a user via POST /test/seed-user and returns email + password.
func seedUser(t *testing.T, role string) (email, password string) {
	t.Helper()
	tc := newTestClient(t)
	email = uniqueEmail(role)
	password = "testpass123"

	resp, err := tc.SeedUserWithResponse(context.Background(), testclient.SeedUserJSONRequestBody{
		Email:    openapi_types.Email(email),
		Password: password,
		Role:     testclient.SeedUserRequestRole(role),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	return email, password
}

// getResetToken retrieves the raw reset token via GET /test/reset-tokens?email=...
func getResetToken(t *testing.T, email string) string {
	t.Helper()
	tc := newTestClient(t)

	resp, err := tc.GetResetTokenWithResponse(context.Background(), &testclient.GetResetTokenParams{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Token
}

// loginAs logs in and returns the access token. Also populates the cookie jar with refresh token.
func loginAs(t *testing.T, c *apiclient.ClientWithResponses, email, password string) string {
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

// withAuth adds an Authorization: Bearer header to requests.
func withAuth(token string) apiclient.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}
