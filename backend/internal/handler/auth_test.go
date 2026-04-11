package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// --- helpers ---

type apiResponse struct {
	Data  json.RawMessage  `json:"data,omitempty"`
	Error *domain.APIError `json:"error,omitempty"`
}

func doRequest(handler http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, "/", nil)
	}
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func parseResponse(t *testing.T, w *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var resp apiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp),
		"failed to parse response body: %s", w.Body.String())
	return resp
}

// --- Login handler tests ---

func TestLoginHandler_Success(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	auth.EXPECT().Login(mock.Anything, "test@example.com", "password123").
		Return(&service.LoginResult{
			AccessToken:      "jwt-token",
			RefreshTokenRaw:  "refresh-raw",
			RefreshExpiresAt: 1234567890,
			User:             repository.UserRow{ID: "u-1", Email: "test@example.com", Role: "admin"},
		}, nil)

	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.Login, "POST", `{"email":"test@example.com","password":"password123"}`)

	require.Equal(t, http.StatusOK, w.Code)

	var refreshCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "refresh_token" {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie, "refresh_token cookie not set")
	assert.Equal(t, "refresh-raw", refreshCookie.Value)
	assert.True(t, refreshCookie.HttpOnly)
}

func TestLoginHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.Login, "POST", `not json`)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestLoginHandler_MissingFields(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.Login, "POST", `{"email":"","password":""}`)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestLoginHandler_ShortPassword(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	auth.EXPECT().Login(mock.Anything, "a@b.com", "12345").
		Return(nil, domain.ErrUnauthorized)

	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.Login, "POST", `{"email":"a@b.com","password":"12345"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLoginHandler_WrongCredentials(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	auth.EXPECT().Login(mock.Anything, "a@b.com", "wrongpass").
		Return(nil, domain.ErrUnauthorized)

	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.Login, "POST", `{"email":"a@b.com","password":"wrongpass"}`)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseResponse(t, w)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "UNAUTHORIZED", resp.Error.Code)
}

func TestLoginHandler_EmailNormalization(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	// Mock expects normalized email — if handler doesn't normalize, AssertExpectations fails
	auth.EXPECT().Login(mock.Anything, "admin@example.com", "password123").
		Return(nil, domain.ErrUnauthorized)

	h := NewAuthHandler(auth, nil, false)
	doRequest(h.Login, "POST", `{"email":"  Admin@Example.COM  ","password":"password123"}`)
}

// --- Refresh handler tests ---

func TestRefreshHandler_NoCookie(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)

	r := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	h.Refresh(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Password reset request tests ---

func TestPasswordResetRequest_AlwaysReturns200(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	auth.EXPECT().RequestPasswordReset(mock.Anything, "anyone@test.com").Return(nil)

	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.RequestPasswordReset, "POST", `{"email":"anyone@test.com"}`)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPasswordResetRequest_EmptyEmail(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.RequestPasswordReset, "POST", `{"email":""}`)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- ResetPassword handler tests ---

func TestResetPassword_Success(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	auth.EXPECT().ResetPassword(mock.Anything, "abc", "newpass123").Return("user-1", nil)

	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.ResetPassword, "POST", `{"token":"abc","newPassword":"newpass123"}`)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResetPassword_MissingToken(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.ResetPassword, "POST", `{"token":"","newPassword":"newpass123"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestResetPassword_ShortPassword(t *testing.T) {
	t.Parallel()
	auth := mocks.NewMockAuth(t)
	h := NewAuthHandler(auth, nil, false)
	w := doRequest(h.ResetPassword, "POST", `{"token":"abc","newPassword":"12345"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}
