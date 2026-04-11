package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware/mocks"
)

func setRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ContextKeyRole, role)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func parseError(t *testing.T, w *httptest.ResponseRecorder) domain.APIResponse {
	t.Helper()
	var resp domain.APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// --- Auth middleware tests ---

func TestAuth_ValidToken(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	v.EXPECT().ValidateAccessToken("good-token").Return("user-1", "admin", nil)

	handler := Auth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "user-1", UserIDFromContext(r.Context()))
		assert.Equal(t, "admin", RoleFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer good-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_MissingHeader(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	handler := Auth(v)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "UNAUTHORIZED", resp.Error.Code)
}

func TestAuth_InvalidHeaderFormat(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	handler := Auth(v)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Token abc")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_BearerOnly(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	handler := Auth(v)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_InvalidToken(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	v.EXPECT().ValidateAccessToken("bad-token").Return("", "", fmt.Errorf("expired"))

	handler := Auth(v)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer bad-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "UNAUTHORIZED", resp.Error.Code)
}

func TestAuth_CaseInsensitiveBearer(t *testing.T) {
	t.Parallel()
	v := mocks.NewMockTokenValidator(t)
	v.EXPECT().ValidateAccessToken("tok").Return("u-1", "admin", nil)

	handler := Auth(v)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "bearer tok")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

// --- RequireRole tests ---

func TestRequireRole_Allowed(t *testing.T) {
	t.Parallel()
	handler := RequireRole("admin", "brand_manager")(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	ctx := r.Context()
	ctx = setRole(ctx, "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(ctx))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_Denied(t *testing.T) {
	t.Parallel()
	handler := RequireRole("admin")(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	ctx := setRole(r.Context(), "brand_manager")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(ctx))

	assert.Equal(t, http.StatusForbidden, w.Code)
	resp := parseError(t, w)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
}

func TestRequireRole_EmptyRole(t *testing.T) {
	t.Parallel()
	handler := RequireRole("admin")(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Context helpers tests ---

func TestContextHelpers_Empty(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	assert.Equal(t, "", UserIDFromContext(r.Context()))
	assert.Equal(t, "", RoleFromContext(r.Context()))
}
