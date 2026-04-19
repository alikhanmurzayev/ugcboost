package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware/mocks"
)

func setRole(ctx context.Context, role api.UserRole) context.Context {
	return context.WithValue(ctx, ContextKeyRole, role)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func parseError(t *testing.T, w *httptest.ResponseRecorder) api.ErrorResponse {
	t.Helper()
	var resp api.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestAuth_ValidateAccessToken(t *testing.T) {
	t.Parallel()

	t.Run("valid token", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		v.EXPECT().ValidateAccessToken("good-token").Return("user-1", "admin", nil)

		var gotUserID string
		var gotRole api.UserRole
		handler := Auth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUserID = UserIDFromContext(r.Context())
			gotRole = RoleFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer good-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "user-1", gotUserID)
		require.Equal(t, api.Admin, gotRole)
	})

	t.Run("missing header", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		resp := parseError(t, w)
		require.Equal(t, "UNAUTHORIZED", resp.Error.Code)
	})

	t.Run("invalid header format", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Token abc")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("bearer only", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		v.EXPECT().ValidateAccessToken("bad-token").Return("", "", fmt.Errorf("expired"))

		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer bad-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		resp := parseError(t, w)
		require.Equal(t, "UNAUTHORIZED", resp.Error.Code)
	})

	t.Run("case insensitive bearer", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		v.EXPECT().ValidateAccessToken("tok").Return("u-1", "admin", nil)

		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "bearer tok")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRequireRole_Check(t *testing.T) {
	t.Parallel()

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		handler := RequireRole(api.Admin, api.BrandManager)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		ctx := setRole(r.Context(), api.Admin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()
		handler := RequireRole(api.Admin)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		ctx := setRole(r.Context(), api.BrandManager)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusForbidden, w.Code)
		resp := parseError(t, w)
		require.Equal(t, "FORBIDDEN", resp.Error.Code)
	})

	t.Run("empty role", func(t *testing.T) {
		t.Parallel()
		handler := RequireRole(api.Admin)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestContextHelpers_Empty(t *testing.T) {
	t.Parallel()

	t.Run("returns empty strings", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest("GET", "/", nil)
		require.Equal(t, "", UserIDFromContext(r.Context()))
		require.Equal(t, api.UserRole(""), RoleFromContext(r.Context()))
	})
}
