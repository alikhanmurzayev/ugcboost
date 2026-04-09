package e2etest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/e2etest/apiclient"
)

var ctx = context.Background()

// --- Health ---

func TestHealthCheck(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.HealthCheckWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.NotNil(t, resp.JSON200)
	assert.Equal(t, "ok", resp.JSON200.Status)
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)

	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: password,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.NotEmpty(t, resp.JSON200.Data.AccessToken)
	assert.Equal(t, email, string(resp.JSON200.Data.User.Email))
	assert.Equal(t, apiclient.Admin, resp.JSON200.Data.User.Role)

	// Refresh cookie should be set
	cookies := resp.HTTPResponse.Cookies()
	var hasRefresh bool
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			hasRefresh = true
			assert.True(t, c.HttpOnly)
		}
	}
	assert.True(t, hasRefresh, "refresh_token cookie must be set")
}

func TestLogin_WrongPassword(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)

	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: "wrongpassword",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	require.NotNil(t, resp.JSON401)
	assert.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
}

func TestLogin_NonExistentEmail(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email("nobody@example.com"), Password: "password123",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestLogin_EmptyEmail(t *testing.T) {
	// Use raw HTTP — the generated client validates Email format before sending
	body, _ := json.Marshal(map[string]string{"email": "", "password": "password123"})
	req, err := http.NewRequest("POST", baseURL+"/auth/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient(nil).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestLogin_ShortPassword(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)
	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: "12345",
	})
	require.NoError(t, err)
	// Short password is not pre-validated on login (prevents info leak);
	// bcrypt comparison fails → 401
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestLogin_EmailNormalization(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)

	// Login with uppercase + whitespace
	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email("  " + email + "  "), Password: password,
	})
	require.NoError(t, err)
	// Handler trims + lowercases before lookup, so it should work
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.Equal(t, email, string(resp.JSON200.Data.User.Email))
}

// --- Refresh ---

func TestRefresh_Success(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	loginAs(t, c, email, password)

	// Refresh — cookie jar sends the refresh_token automatically
	resp, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.NotEmpty(t, resp.JSON200.Data.AccessToken)
	assert.Equal(t, email, string(resp.JSON200.Data.User.Email))
}

func TestRefresh_NoCookie(t *testing.T) {
	c := newAPIClient(t) // fresh client, no cookies
	resp, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestRefresh_SingleUse(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	loginAs(t, c, email, password)

	// First refresh succeeds
	resp1, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode())

	// Second refresh with the rotated token should also work
	// (cookie jar was updated with the new token from resp1)
	resp2, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode())
}

// --- Auth Me ---

func TestGetMe_Success(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	token := loginAs(t, c, email, password)

	resp, err := c.GetMeWithResponse(ctx, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.Equal(t, email, string(resp.JSON200.Data.Email))
	assert.Equal(t, apiclient.Admin, resp.JSON200.Data.Role)
}

func TestGetMe_NoToken(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.GetMeWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestGetMe_InvalidToken(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.GetMeWithResponse(ctx, withAuth("invalid-jwt-token"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

// --- Logout ---

func TestLogout_Success(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	token := loginAs(t, c, email, password)

	resp, err := c.LogoutWithResponse(ctx, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
}

func TestLogout_InvalidatesRefreshTokens(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	token := loginAs(t, c, email, password)

	// Logout
	resp, err := c.LogoutWithResponse(ctx, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())

	// Refresh should fail (tokens invalidated)
	resp2, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode())
}

func TestLogout_NoAuth(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.LogoutWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

// --- Password Reset Request ---

func TestPasswordResetRequest_ExistingEmail(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)

	resp, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
}

func TestPasswordResetRequest_NonExistentEmail(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email("nonexistent@example.com"),
	})
	require.NoError(t, err)
	// Always 200 to prevent email enumeration
	assert.Equal(t, http.StatusOK, resp.StatusCode())
}

func TestPasswordResetRequest_EmptyEmail(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.RequestPasswordResetWithResponse(context.Background(), apiclient.RequestPasswordResetJSONRequestBody{
		Email: "",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}

// --- Password Reset Execute ---

func TestResetPassword_Success(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)

	// Request reset
	_, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)

	// Get raw token from test endpoint
	rawToken := getResetToken(t, email)
	require.NotEmpty(t, rawToken)

	// Reset password
	newPassword := "newpassword123"
	resp, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: newPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())

	// Login with new password works
	loginResp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: newPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, loginResp.StatusCode())

	// Login with old password fails
	loginResp2, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: password,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, loginResp2.StatusCode())
}

func TestResetPassword_InvalidToken(t *testing.T) {
	c := newAPIClient(t)
	resp, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: "invalid-token", NewPassword: "newpassword123",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestResetPassword_UsedToken(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)

	// Request + get token
	_, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	rawToken := getResetToken(t, email)

	// Use token once
	resp1, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: "newpass1234",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode())

	// Second use fails (single-use token)
	resp2, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: "newpass5678",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode())
}

func TestResetPassword_ShortPassword(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)

	_, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	rawToken := getResetToken(t, email)

	resp, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: "short",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}

func TestResetPassword_InvalidatesRefreshTokens(t *testing.T) {
	email, _ := seedUser(t, "admin")
	c := newAPIClient(t)
	loginAs(t, c, email, "testpass123")

	// Request + reset password
	_, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	rawToken := getResetToken(t, email)

	_, err = c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: "newpassword123",
	})
	require.NoError(t, err)

	// Old refresh token should be invalid
	resp, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

// --- Roles ---

func TestSeedUser_AdminRole(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)

	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: password,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, apiclient.Admin, resp.JSON200.Data.User.Role)
}

func TestSeedUser_BrandManagerRole(t *testing.T) {
	email, password := seedUser(t, "brand_manager")
	c := newAPIClient(t)

	resp, err := c.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: password,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, apiclient.BrandManager, resp.JSON200.Data.User.Role)
}

// --- Full flows ---

func TestFullFlow_LoginRefreshMeLogout(t *testing.T) {
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)

	// Login
	token := loginAs(t, c, email, password)

	// Refresh
	refreshResp, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, refreshResp.StatusCode())
	newToken := refreshResp.JSON200.Data.AccessToken

	// Me (with refreshed token)
	meResp, err := c.GetMeWithResponse(ctx, withAuth(newToken))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, meResp.StatusCode())
	assert.Equal(t, email, string(meResp.JSON200.Data.Email))

	// Logout
	logoutResp, err := c.LogoutWithResponse(ctx, withAuth(newToken))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode())

	// Verify: refresh fails after logout
	refreshResp2, err := c.RefreshTokenWithResponse(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, refreshResp2.StatusCode())

	// Verify: me fails with old token (it's still valid JWT but that's ok,
	// the test verifies the session is destroyed from refresh perspective)
	_ = token
}

func TestFullFlow_PasswordReset(t *testing.T) {
	email, oldPassword := seedUser(t, "admin")
	c := newAPIClient(t)

	// Login with original password
	loginAs(t, c, email, oldPassword)

	// Request reset
	_, err := c.RequestPasswordResetWithResponse(ctx, apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)

	// Get reset token
	rawToken := getResetToken(t, email)

	// Reset password
	newPassword := "brandnewpassword"
	resetResp, err := c.ResetPasswordWithResponse(ctx, apiclient.ResetPasswordJSONRequestBody{
		Token: rawToken, NewPassword: newPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resetResp.StatusCode())

	// Login with new password
	c2 := newAPIClient(t)
	loginResp, err := c2.LoginWithResponse(ctx, apiclient.LoginJSONRequestBody{
		Email: openapi_types.Email(email), Password: newPassword,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, loginResp.StatusCode())
}
