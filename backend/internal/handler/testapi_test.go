package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// newTestAPIRouter registers a TestAPIHandler behind the generated testapi wrapper.
func newTestAPIRouter(t *testing.T, h *TestAPIHandler) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	testapi.HandlerFromMux(h, r)
	return r
}

const seedAdminID = "admin-seed-id"

func expectUnexpectedErrorLog(log *logmocks.MockLogger, path string) {
	log.EXPECT().Error(mock.Anything, "unexpected error", mock.MatchedBy(func(args []any) bool {
		return len(args) == 4 && args[0] == "error" && args[2] == "path" && args[3] == path
	})).Once()
}

func TestTestAPIHandler_SeedUser(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			map[string]any{"email": 123})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{Email: "user@example.com", Password: "pass"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		auth.EXPECT().SeedUser(mock.Anything, "user@example.com", "pass", "admin").
			Return(nil, errors.New("db error"))
		expectUnexpectedErrorLog(log, "/test/seed-user")
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{
				Email: "user@example.com", Password: "pass",
				Role: testapi.SeedUserRequestRoleAdmin,
			})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		auth.EXPECT().SeedUser(mock.Anything, "user@example.com", "pass", "admin").
			Return(&domain.User{ID: "u-seed", Email: "user@example.com", Role: api.Admin}, nil)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[testapi.SeedUserResult](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{
				Email: "user@example.com", Password: "pass",
				Role: testapi.SeedUserRequestRoleAdmin,
			})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, testapi.SeedUserResult{
			Data: testapi.SeedUserData{
				Id:    "u-seed",
				Email: openapi_types.Email("user@example.com"),
				Role:  testapi.SeedUserDataRoleAdmin,
			},
		}, resp)
	})
}

func TestTestAPIHandler_SeedBrand(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-brand",
			map[string]any{"name": 42})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-brand",
			testapi.SeedBrandRequest{Name: ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("brand create error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
			Return(nil, errors.New("db error"))
		expectUnexpectedErrorLog(log, "/test/seed-brand")
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-brand",
			testapi.SeedBrandRequest{Name: "Test Brand"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success without managerEmail impersonates admin", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
			Run(func(ctx context.Context, _ string, _ *string) {
				// Impersonation: handler must write adminID + Admin role before
				// calling the brand service so audit rows have a valid actor.
				require.Equal(t, seedAdminID, middleware.UserIDFromContext(ctx))
				require.Equal(t, api.Admin, middleware.RoleFromContext(ctx))
			}).
			Return(&domain.Brand{ID: "b-seed", Name: "Test Brand"}, nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[testapi.SeedBrandResult](t, router, http.MethodPost, "/test/seed-brand",
			testapi.SeedBrandRequest{Name: "Test Brand"})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, testapi.SeedBrandResult{
			Data: testapi.SeedBrandData{Id: "b-seed", Name: "Test Brand"},
		}, resp)
	})

	t.Run("success with managerEmail assigns manager", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		managerEmail := openapi_types.Email("mgr@example.com")

		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
			Return(&domain.Brand{ID: "b-seed", Name: "Test Brand"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-seed", "mgr@example.com").
			Return(&domain.User{ID: "u-mgr", Email: "mgr@example.com"}, "tmp-pass", nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[testapi.SeedBrandResult](t, router, http.MethodPost, "/test/seed-brand",
			testapi.SeedBrandRequest{Name: "Test Brand", ManagerEmail: &managerEmail})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, testapi.SeedBrandResult{
			Data: testapi.SeedBrandData{Id: "b-seed", Name: "Test Brand"},
		}, resp)
	})

	t.Run("assign manager error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		managerEmail := openapi_types.Email("mgr@example.com")

		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
			Return(&domain.Brand{ID: "b-seed", Name: "Test Brand"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-seed", "mgr@example.com").
			Return(nil, "", errors.New("assign failed"))
		expectUnexpectedErrorLog(log, "/test/seed-brand")

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-brand",
			testapi.SeedBrandRequest{Name: "Test Brand", ManagerEmail: &managerEmail})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestTestAPIHandler_GetResetToken(t *testing.T) {
	t.Parallel()

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		store.EXPECT().GetToken("missing@example.com").Return("", false)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet,
			"/test/reset-tokens?email=missing@example.com", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		brands := mocks.NewMockTestAPIBrandService(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		store.EXPECT().GetToken("alice@example.com").Return("raw-token-123", true)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, brands, store, seedAdminID, log))

		w, resp := doJSON[testapi.ResetTokenResult](t, router, http.MethodGet,
			"/test/reset-tokens?email=alice@example.com", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, testapi.ResetTokenResult{
			Data: testapi.ResetTokenData{Token: "raw-token-123"},
		}, resp)
	})
}
