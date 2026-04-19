package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
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
		v.EXPECT().ValidateAccessToken("bad-token").Return("", "", errors.New("expired"))

		handler := Auth(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer bad-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, api.ErrorResponse{
			Error: api.APIError{
				Code:    domain.CodeUnauthorized,
				Message: "Invalid or expired token",
			},
		}, parseError(t, w))
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

func TestAuth_FromScopes(t *testing.T) {
	t.Parallel()

	t.Run("no BearerAuthScopes in context skips auth", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)

		called := false
		handler := AuthFromScopes(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			// Without scopes, auth is bypassed — user id / role remain empty.
			require.Empty(t, UserIDFromContext(r.Context()))
			require.Empty(t, string(RoleFromContext(r.Context())))
			w.WriteHeader(http.StatusOK)
		}))

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		require.True(t, called)
	})

	t.Run("with scopes enforces auth and passes claims to next handler", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		v.EXPECT().ValidateAccessToken("good").Return("user-9", "admin", nil)

		handler := AuthFromScopes(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "user-9", UserIDFromContext(r.Context()))
			require.Equal(t, api.Admin, RoleFromContext(r.Context()))
			w.WriteHeader(http.StatusOK)
		}))

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer good")
		//nolint:staticcheck // SA1029: api.BearerAuthScopes is the exact key the generated server wrapper sets; tests must use the same.
		ctx := context.WithValue(r.Context(), api.BearerAuthScopes, []string{})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("with scopes rejects missing header", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		handler := AuthFromScopes(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		//nolint:staticcheck // SA1029: api.BearerAuthScopes is the exact key the generated server wrapper sets; tests must use the same.
		ctx := context.WithValue(r.Context(), api.BearerAuthScopes, []string{})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, api.ErrorResponse{
			Error: api.APIError{
				Code:    domain.CodeUnauthorized,
				Message: "Missing authorization header",
			},
		}, parseError(t, w))
	})

	t.Run("with scopes rejects malformed header", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		handler := AuthFromScopes(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Token abc")
		//nolint:staticcheck // SA1029: api.BearerAuthScopes is the exact key the generated server wrapper sets; tests must use the same.
		ctx := context.WithValue(r.Context(), api.BearerAuthScopes, []string{})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("with scopes rejects invalid token", func(t *testing.T) {
		t.Parallel()
		v := mocks.NewMockTokenValidator(t)
		v.EXPECT().ValidateAccessToken("bad").Return("", "", errors.New("expired"))
		handler := AuthFromScopes(v)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer bad")
		//nolint:staticcheck // SA1029: api.BearerAuthScopes is the exact key the generated server wrapper sets; tests must use the same.
		ctx := context.WithValue(r.Context(), api.BearerAuthScopes, []string{})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r.WithContext(ctx))

		require.Equal(t, http.StatusUnauthorized, w.Code)
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
