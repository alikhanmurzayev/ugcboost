package testutil

import (
	"context"
	"fmt"
	"math/rand"
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
	runID   = fmt.Sprintf("%d%04d", time.Now().Unix(), rand.Intn(10000))
)

// UniqueEmail generates a unique email for test isolation.
func UniqueEmail(prefix string) string {
	n := atomic.AddUint64(&counter, 1)
	return fmt.Sprintf("test-%s-%s-%d@e2e.test", prefix, runID, n)
}

// SeedUser creates a user via POST /test/seed-user and returns email + password.
func SeedUser(t *testing.T, role string) (email, password string) {
	t.Helper()
	tc := NewTestClient(t)
	email = UniqueEmail(role)
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

// SeedBrand creates a brand via POST /test/seed-brand and returns brandID.
func SeedBrand(t *testing.T, name string) string {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.SeedBrandWithResponse(context.Background(), testclient.SeedBrandJSONRequestBody{
		Name: name,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id
}

// SeedBrandWithManager creates a brand with a manager via POST /test/seed-brand.
func SeedBrandWithManager(t *testing.T, name, managerEmail string) string {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.SeedBrandWithResponse(context.Background(), testclient.SeedBrandJSONRequestBody{
		Name:         name,
		ManagerEmail: (*openapi_types.Email)(&managerEmail),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id
}

// LoginAsAdmin seeds an admin user and returns an authenticated API client with token.
func LoginAsAdmin(t *testing.T) (*apiclient.ClientWithResponses, string) {
	t.Helper()
	email, password := SeedUser(t, "admin")
	c := NewAPIClient(t)
	token := LoginAs(t, c, email, password)
	return c, token
}

// LoginAsBrandManager seeds a brand_manager user and returns client, token, email, password.
func LoginAsBrandManager(t *testing.T) (*apiclient.ClientWithResponses, string, string, string) {
	t.Helper()
	email, password := SeedUser(t, "brand_manager")
	c := NewAPIClient(t)
	token := LoginAs(t, c, email, password)
	return c, token, email, password
}
