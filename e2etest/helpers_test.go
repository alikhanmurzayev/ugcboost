package e2etest

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"sync/atomic"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/e2etest/apiclient"
	"github.com/alikhanmurzayev/ugcboost/e2etest/testclient"
)

const baseURL = "http://localhost:8082"

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
		apiclient.WithHTTPClient(&http.Client{Jar: jar}))
	require.NoError(t, err)
	return c
}

// newTestClient creates a client for test-only endpoints.
func newTestClient(t *testing.T) *testclient.ClientWithResponses {
	t.Helper()
	c, err := testclient.NewClientWithResponses(baseURL)
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
