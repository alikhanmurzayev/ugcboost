package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

func TestServer_Login(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "test@example.com", "password123").
			Return(&service.LoginResult{
				AccessToken:      "jwt-token",
				RefreshTokenRaw:  "refresh-raw",
				RefreshExpiresAt: 1234567890,
				User:             repository.UserRow{ID: "u-1", Email: "test@example.com", Role: "admin"},
			}, nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		body := `{"email":"test@example.com","password":"password123"}`
		r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var refreshCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "refresh_token" {
				refreshCookie = c
			}
		}
		require.NotNil(t, refreshCookie, "refresh_token cookie not set")
		require.Equal(t, "refresh-raw", refreshCookie.Value)
		require.True(t, refreshCookie.HttpOnly)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader("not json"))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"","password":""}`))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("short password", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "a@b.com", "12345").
			Return(nil, domain.ErrUnauthorized)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@b.com","password":"12345"}`))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong credentials", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "a@b.com", "wrongpass").
			Return(nil, domain.ErrUnauthorized)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@b.com","password":"wrongpass"}`))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("email normalization", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Login(mock.Anything, "admin@example.com", "password123").
			Return(nil, domain.ErrUnauthorized)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"  Admin@Example.COM  ","password":"password123"}`))
		r.Header.Set("Content-Type", "application/json")
		s.Login(w, r)
		// If handler normalizes, mock will match on normalized email
	})
}

func TestServer_RefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("no cookie", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		s.RefreshToken(w, r)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Refresh(mock.Anything, "valid-refresh").
			Return(&service.RefreshResult{
				AccessToken:      "new-access",
				RefreshTokenRaw:  "new-refresh",
				RefreshExpiresAt: 9999999999,
				User:             repository.UserRow{ID: "u-1", Email: "test@example.com", Role: "admin"},
			}, nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "valid-refresh"})
		s.RefreshToken(w, r)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestServer_RequestPasswordReset(t *testing.T) {
	t.Parallel()

	t.Run("always returns 200", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().RequestPasswordReset(mock.Anything, "anyone@test.com").Return(nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"anyone@test.com"}`))
		r.Header.Set("Content-Type", "application/json")
		s.RequestPasswordReset(w, r)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty email", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":""}`))
		r.Header.Set("Content-Type", "application/json")
		s.RequestPasswordReset(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})
}

func TestServer_ResetPassword(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().ResetPassword(mock.Anything, "abc", "newpass123").Return("user-1", nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"abc","newPassword":"newpass123"}`))
		r.Header.Set("Content-Type", "application/json")
		s.ResetPassword(w, r)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing token", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"","newPassword":"newpass123"}`))
		r.Header.Set("Content-Type", "application/json")
		s.ResetPassword(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("short password", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"abc","newPassword":"12345"}`))
		r.Header.Set("Content-Type", "application/json")
		s.ResetPassword(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})
}

func TestServer_Logout(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().Logout(mock.Anything, "user-1").Return(nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, "user-1")
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "admin")
		r = r.WithContext(ctx)
		s.Logout(w, r)
		require.Equal(t, http.StatusOK, w.Code)

		// Verify refresh cookie is cleared
		var refreshCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "refresh_token" {
				refreshCookie = c
			}
		}
		require.NotNil(t, refreshCookie)
		require.Equal(t, "", refreshCookie.Value)
		require.Equal(t, -1, refreshCookie.MaxAge)
	})

	t.Run("no user in context", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		s.Logout(w, r)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestServer_GetMe(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockAuthService(t)
		auth.EXPECT().GetUser(mock.Anything, "user-1").
			Return(&repository.UserRow{ID: "user-1", Email: "test@example.com", Role: "admin"}, nil)

		s := NewServer(auth, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, "user-1")
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "admin")
		r = r.WithContext(ctx)
		s.GetMe(w, r)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("no user in context", func(t *testing.T) {
		t.Parallel()
		s := NewServer(mocks.NewMockAuthService(t), nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		s.GetMe(w, r)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
