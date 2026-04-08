package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// --- mock auth service ---

type mockAuth struct {
	login              func(ctx context.Context, email, password string) (*service.LoginResult, error)
	refresh            func(ctx context.Context, rawToken string) (*service.RefreshResult, error)
	logout             func(ctx context.Context, userID string) error
	requestPasswordReset func(ctx context.Context, email string) error
	resetPassword      func(ctx context.Context, rawToken, newPassword string) error
	getUser            func(ctx context.Context, userID string) (repository.UserRow, error)
}

func (m *mockAuth) Login(ctx context.Context, email, password string) (*service.LoginResult, error) {
	return m.login(ctx, email, password)
}
func (m *mockAuth) Refresh(ctx context.Context, rawToken string) (*service.RefreshResult, error) {
	return m.refresh(ctx, rawToken)
}
func (m *mockAuth) Logout(ctx context.Context, userID string) error {
	return m.logout(ctx, userID)
}
func (m *mockAuth) RequestPasswordReset(ctx context.Context, email string) error {
	return m.requestPasswordReset(ctx, email)
}
func (m *mockAuth) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	return m.resetPassword(ctx, rawToken, newPassword)
}
func (m *mockAuth) GetUser(ctx context.Context, userID string) (repository.UserRow, error) {
	return m.getUser(ctx, userID)
}

// --- helpers ---

func newHandler(auth *mockAuth) *AuthHandler {
	return NewAuthHandler(auth, false)
}

type apiResponse struct {
	Data  json.RawMessage `json:"data,omitempty"`
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
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

// --- Login handler tests ---

func TestLoginHandler_Success(t *testing.T) {
	h := newHandler(&mockAuth{
		login: func(_ context.Context, email, password string) (*service.LoginResult, error) {
			if email != "test@example.com" || password != "password123" {
				t.Errorf("unexpected credentials: %q / %q", email, password)
			}
			return &service.LoginResult{
				AccessToken:      "jwt-token",
				RefreshTokenRaw:  "refresh-raw",
				RefreshExpiresAt: 1234567890,
				User: repository.UserRow{
					ID: "u-1", Email: "test@example.com", Role: "admin",
				},
			}, nil
		},
	})

	w := doRequest(h.Login, "POST", `{"email":"test@example.com","password":"password123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check refresh cookie was set
	cookies := w.Result().Cookies()
	var refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refreshCookie = c
		}
	}
	if refreshCookie == nil {
		t.Fatal("refresh_token cookie not set")
	}
	if refreshCookie.Value != "refresh-raw" {
		t.Errorf("unexpected cookie value: %q", refreshCookie.Value)
	}
	if !refreshCookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
}

func TestLoginHandler_InvalidJSON(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.Login, "POST", `not json`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestLoginHandler_MissingFields(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.Login, "POST", `{"email":"","password":""}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestLoginHandler_ShortPassword(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.Login, "POST", `{"email":"a@b.com","password":"12345"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestLoginHandler_WrongCredentials(t *testing.T) {
	h := newHandler(&mockAuth{
		login: func(_ context.Context, _, _ string) (*service.LoginResult, error) {
			return nil, domain.ErrUnauthorized
		},
	})
	w := doRequest(h.Login, "POST", `{"email":"a@b.com","password":"wrongpass"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	resp := parseResponse(t, w)
	if resp.Error == nil || resp.Error.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED error, got %+v", resp.Error)
	}
}

func TestLoginHandler_EmailNormalization(t *testing.T) {
	var receivedEmail string
	h := newHandler(&mockAuth{
		login: func(_ context.Context, email, _ string) (*service.LoginResult, error) {
			receivedEmail = email
			return nil, domain.ErrUnauthorized
		},
	})

	doRequest(h.Login, "POST", `{"email":"  Admin@Example.COM  ","password":"password123"}`)

	if receivedEmail != "admin@example.com" {
		t.Errorf("email not normalized: got %q", receivedEmail)
	}
}

// --- Refresh handler tests ---

func TestRefreshHandler_NoCookie(t *testing.T) {
	h := newHandler(&mockAuth{})
	r := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	h.Refresh(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// --- Password reset request tests ---

func TestPasswordResetRequest_AlwaysReturns200(t *testing.T) {
	h := newHandler(&mockAuth{
		requestPasswordReset: func(_ context.Context, _ string) error {
			return nil
		},
	})

	w := doRequest(h.RequestPasswordReset, "POST", `{"email":"anyone@test.com"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasswordResetRequest_EmptyEmail(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.RequestPasswordReset, "POST", `{"email":""}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

// --- ResetPassword handler tests ---

func TestResetPassword_Success(t *testing.T) {
	h := newHandler(&mockAuth{
		resetPassword: func(_ context.Context, token, password string) error {
			if token == "" || password == "" {
				t.Error("empty token or password")
			}
			return nil
		},
	})

	w := doRequest(h.ResetPassword, "POST", `{"token":"abc","newPassword":"newpass123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResetPassword_MissingToken(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.ResetPassword, "POST", `{"token":"","newPassword":"newpass123"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestResetPassword_ShortPassword(t *testing.T) {
	h := newHandler(&mockAuth{})
	w := doRequest(h.ResetPassword, "POST", `{"token":"abc","newPassword":"12345"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}
