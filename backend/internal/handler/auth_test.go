package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

func newTestUser() domain.User {
	return domain.User{ID: "u-1", Email: "user@example.com", Role: api.Admin}
}

// refreshCookie pulls the refresh_token cookie from the response, failing the
// test if it is absent.
func refreshCookie(t *testing.T, w *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range w.Cookies() {
		if c.Name == CookieRefreshToken {
			return c
		}
	}
	t.Fatalf("refresh_token cookie not set")
	return nil
}

func TestServer_Login(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/login", map[string]any{
			"email":    "not-an-email",
			"password": 12345,
		})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/login",
			map[string]any{"email": "", "password": ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("wrong credentials", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "a@b.com", "wrongpass").
			Return(nil, domain.ErrUnauthorized)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/login",
			api.LoginRequest{Email: "a@b.com", Password: "wrongpass"})
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, domain.CodeUnauthorized, resp.Error.Code)
	})

	t.Run("email normalization", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "admin@example.com", "password123").
			Return(nil, domain.ErrUnauthorized)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/login",
			map[string]any{"email": "  Admin@Example.COM  ", "password": "password123"})
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := newTestUser()
		refreshExpires := time.Now().Add(7 * 24 * time.Hour)

		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "user@example.com", "password123").
			Return(&service.LoginResult{
				AccessToken:      "jwt-token",
				RefreshTokenRaw:  "refresh-raw",
				RefreshExpiresAt: refreshExpires.Unix(),
				User:             user,
			}, nil)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.LoginResult](t, router, http.MethodPost, "/auth/login",
			api.LoginRequest{Email: "user@example.com", Password: "password123"})

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.LoginResult{
			Data: api.LoginData{
				AccessToken: "jwt-token",
				User:        api.User{Id: user.ID, Email: openapi_types.Email(user.Email), Role: user.Role},
			},
		}, resp)

		cookie := refreshCookie(t, w.Result())
		require.Equal(t, "refresh-raw", cookie.Value)
		require.True(t, cookie.HttpOnly)
		require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
		require.WithinDuration(t, refreshExpires, cookie.Expires, time.Second)
	})
}

func TestServer_RefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("no cookie", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/refresh", nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, domain.CodeUnauthorized, resp.Error.Code)
	})

	t.Run("service unauthorized", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Refresh(mock.Anything, "stale").
			Return(nil, domain.ErrUnauthorized)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/refresh", nil,
			func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "stale"})
			})
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, domain.CodeUnauthorized, resp.Error.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := newTestUser()
		refreshExpires := time.Now().Add(30 * 24 * time.Hour)

		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Refresh(mock.Anything, "valid-refresh").
			Return(&service.RefreshResult{
				AccessToken:      "new-access",
				RefreshTokenRaw:  "new-refresh",
				RefreshExpiresAt: refreshExpires.Unix(),
				User:             user,
			}, nil)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.LoginResult](t, router, http.MethodPost, "/auth/refresh", nil,
			func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "valid-refresh"})
			})
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.LoginResult{
			Data: api.LoginData{
				AccessToken: "new-access",
				User:        api.User{Id: user.ID, Email: openapi_types.Email(user.Email), Role: user.Role},
			},
		}, resp)

		cookie := refreshCookie(t, w.Result())
		require.Equal(t, "new-refresh", cookie.Value)
		require.WithinDuration(t, refreshExpires, cookie.Expires, time.Second)
	})
}

func TestServer_RequestPasswordReset(t *testing.T) {
	t.Parallel()

	t.Run("empty email", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/password-reset-request",
			map[string]any{"email": ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("always returns 200 even when service logs error", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		// Even a service failure must not leak existence. Log happens, response stays 200.
		auth.EXPECT().RequestPasswordReset(mock.Anything, "anyone@test.com").
			Return(errors.New("db error"))

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "password reset request failed", mock.MatchedBy(func(args []any) bool {
			return len(args) == 2 && args[0] == "error"
		})).Once()

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},log))

		w, resp := doJSON[api.MessageResponse](t, router, http.MethodPost, "/auth/password-reset-request",
			api.PasswordResetRequestBody{Email: "anyone@test.com"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{
			Data: api.MessageData{Message: "If the email exists, a reset link has been sent"},
		}, resp)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().RequestPasswordReset(mock.Anything, "anyone@test.com").Return(nil)

		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.MessageResponse](t, router, http.MethodPost, "/auth/password-reset-request",
			api.PasswordResetRequestBody{Email: "anyone@test.com"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{
			Data: api.MessageData{Message: "If the email exists, a reset link has been sent"},
		}, resp)
	})
}

func TestServer_ResetPassword(t *testing.T) {
	t.Parallel()

	t.Run("missing token", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/password-reset",
			api.PasswordResetBody{Token: "", NewPassword: "newpass123"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("short password", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/password-reset",
			api.PasswordResetBody{Token: "abc", NewPassword: "12345"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().ResetPassword(mock.Anything, "abc", "newpass123").
			Return("", domain.ErrUnauthorized)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/password-reset",
			api.PasswordResetBody{Token: "abc", NewPassword: "newpass123"})
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, domain.CodeUnauthorized, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().ResetPassword(mock.Anything, "abc", "newpass123").
			Return("", errors.New("db error"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/auth/password-reset")
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/password-reset",
			api.PasswordResetBody{Token: "abc", NewPassword: "newpass123"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().ResetPassword(mock.Anything, "abc", "newpass123").Return("user-1", nil)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.MessageResponse](t, router, http.MethodPost, "/auth/password-reset",
			api.PasswordResetBody{Token: "abc", NewPassword: "newpass123"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{
			Data: api.MessageData{Message: "Password updated successfully"},
		}, resp)
	})
}

func TestServer_Logout(t *testing.T) {
	t.Parallel()

	t.Run("no user in context", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/auth/logout", nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("service error still returns 200 with cleared cookie", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		// Logout handler deliberately logs errors but always clears the cookie
		// and returns 200 — anything else would leak session state.
		auth.EXPECT().Logout(mock.Anything, "u-admin").Return(errors.New("db error"))
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "failed to revoke refresh tokens on logout", mock.MatchedBy(func(args []any) bool {
			return len(args) == 4 && args[0] == "error" && args[2] == "userID" && args[3] == "u-admin"
		})).Once()
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},log))

		w, resp := doJSON[api.MessageResponse](t, router, http.MethodPost, "/auth/logout", nil,
			withRole("u-admin", api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{
			Data: api.MessageData{Message: "Logged out"},
		}, resp)

		cookie := refreshCookie(t, w.Result())
		require.Equal(t, "", cookie.Value)
		require.Equal(t, -1, cookie.MaxAge)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Logout(mock.Anything, "u-admin").Return(nil)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.MessageResponse](t, router, http.MethodPost, "/auth/logout", nil,
			withRole("u-admin", api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{
			Data: api.MessageData{Message: "Logged out"},
		}, resp)

		cookie := refreshCookie(t, w.Result())
		require.Equal(t, "", cookie.Value)
		require.Equal(t, -1, cookie.MaxAge)
		require.True(t, cookie.HttpOnly)
	})
}

func TestServer_GetMe(t *testing.T) {
	t.Parallel()

	t.Run("no user in context", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/auth/me", nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().GetUser(mock.Anything, "u-admin").
			Return(nil, errors.New("db error"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/auth/me")
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/auth/me", nil,
			withRole("u-admin", api.Admin))
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		user := newTestUser()
		auth.EXPECT().GetUser(mock.Anything, "u-admin").Return(&user, nil)
		router := newTestRouter(t, NewServer(auth, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))

		w, resp := doJSON[api.UserResponse](t, router, http.MethodGet, "/auth/me", nil,
			withRole("u-admin", api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.UserResponse{
			Data: api.User{Id: user.ID, Email: openapi_types.Email(user.Email), Role: user.Role},
		}, resp)
	})
}
