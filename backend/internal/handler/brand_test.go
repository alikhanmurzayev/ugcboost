package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
)

func TestServer_CreateBrand(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateBrand(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/brands",
			api.CreateBrandRequest{Name: "Test"})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateBrand(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/brands",
			map[string]any{"name": 123})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("validation error from service", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateBrand(mock.Anything).Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().CreateBrand(mock.Anything, "", (*string)(nil)).
			Return((*domain.Brand)(nil), domain.NewValidationError(domain.CodeValidation, "Brand name is required"))

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/brands",
			api.CreateBrandRequest{Name: ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateBrand(mock.Anything).Return(nil)
		brands := mocks.NewMockBrandService(t)
		created := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		logoURL := "https://cdn.example.com/l.png"
		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", &logoURL).
			Return(&domain.Brand{
				ID: "b-1", Name: "Test Brand", LogoURL: &logoURL,
				CreatedAt: created, UpdatedAt: created,
			}, nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.BrandResult](t, router, http.MethodPost, "/brands",
			api.CreateBrandRequest{Name: "Test Brand", LogoUrl: &logoURL})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.BrandResult{
			Data: api.Brand{
				Id: "b-1", Name: "Test Brand", LogoUrl: &logoURL,
				CreatedAt: created, UpdatedAt: created,
			},
		}, resp)
	})
}

func TestServer_ListBrands(t *testing.T) {
	t.Parallel()

	t.Run("admin sees all", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListBrands(mock.Anything).Return(true, "u-admin", nil)
		brands := mocks.NewMockBrandService(t)
		created := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		brands.EXPECT().ListBrands(mock.Anything, (*string)(nil)).
			Return([]*domain.BrandListItem{
				{ID: "b-1", Name: "Brand 1", ManagerCount: 2, CreatedAt: created, UpdatedAt: created},
			}, nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.ListBrandsResult](t, router, http.MethodGet, "/brands", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.ListBrandsResult{
			Data: api.ListBrandsData{
				Brands: []api.BrandListItem{
					{Id: "b-1", Name: "Brand 1", ManagerCount: 2, CreatedAt: created, UpdatedAt: created},
				},
			},
		}, resp)
	})

	t.Run("authz error propagates as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListBrands(mock.Anything).
			Return(false, "", errors.New("authz failed"))

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/brands", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("service list error propagates as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListBrands(mock.Anything).Return(true, "u-admin", nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().ListBrands(mock.Anything, (*string)(nil)).
			Return(nil, errors.New("db error"))

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/brands", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("manager sees own brands", func(t *testing.T) {
		t.Parallel()
		uid := "u-manager"
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListBrands(mock.Anything).Return(false, uid, nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().ListBrands(mock.Anything, &uid).
			Return([]*domain.BrandListItem{}, nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.ListBrandsResult](t, router, http.MethodGet, "/brands", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.ListBrandsResult{
			Data: api.ListBrandsData{Brands: []api.BrandListItem{}},
		}, resp)
	})
}

func TestServer_GetBrand(t *testing.T) {
	t.Parallel()

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewBrand(mock.Anything, "b-1").Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/brands/b-1", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().GetBrand(mock.Anything, "b-1").
			Return((*domain.Brand)(nil), domain.ErrNotFound)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/brands/b-1", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("list managers error", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().GetBrand(mock.Anything, "b-1").
			Return(&domain.Brand{ID: "b-1", Name: "Test"}, nil)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return(nil, domain.ErrNotFound)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/brands/b-1", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		created := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		assigned := time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC)
		brands.EXPECT().GetBrand(mock.Anything, "b-1").
			Return(&domain.Brand{
				ID: "b-1", Name: "Test", LogoURL: nil,
				CreatedAt: created, UpdatedAt: created,
			}, nil)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return([]*domain.BrandManager{
				{UserID: "u-2", Email: "mgr@example.com", AssignedAt: assigned},
			}, nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.GetBrandResult](t, router, http.MethodGet, "/brands/b-1", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.GetBrandResult{
			Data: api.BrandDetailData{
				Id: "b-1", Name: "Test", LogoUrl: nil,
				Managers: []api.ManagerInfo{
					{UserId: "u-2", Email: "mgr@example.com", AssignedAt: assigned},
				},
				CreatedAt: created, UpdatedAt: created,
			},
		}, resp)
	})
}

func TestServer_UpdateBrand(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateBrand(mock.Anything, "b-1").Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPut, "/brands/b-1",
			api.UpdateBrandRequest{Name: "X"})
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateBrand(mock.Anything, "b-1").Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPut, "/brands/b-1",
			map[string]any{"name": 123})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("service not found", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().UpdateBrand(mock.Anything, "b-1", "X", (*string)(nil)).
			Return((*domain.Brand)(nil), domain.ErrNotFound)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPut, "/brands/b-1",
			api.UpdateBrandRequest{Name: "X"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		updated := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
		brands.EXPECT().UpdateBrand(mock.Anything, "b-1", "New Name", (*string)(nil)).
			Return(&domain.Brand{
				ID: "b-1", Name: "New Name", LogoURL: nil,
				CreatedAt: updated, UpdatedAt: updated,
			}, nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.BrandResult](t, router, http.MethodPut, "/brands/b-1",
			api.UpdateBrandRequest{Name: "New Name"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.BrandResult{
			Data: api.Brand{
				Id: "b-1", Name: "New Name", LogoUrl: nil,
				CreatedAt: updated, UpdatedAt: updated,
			},
		}, resp)
	})
}

func TestServer_DeleteBrand(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanDeleteBrand(mock.Anything, "b-1").Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, "/brands/b-1", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("service error returns 404", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanDeleteBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().DeleteBrand(mock.Anything, "b-1").Return(domain.ErrNotFound)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, "/brands/b-1", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanDeleteBrand(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().DeleteBrand(mock.Anything, "b-1").Return(nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.MessageResponse](t, router, http.MethodDelete, "/brands/b-1", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{Data: api.MessageData{Message: "Brand deleted"}}, resp)
	})
}

func TestServer_AssignManager(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAssignManager(mock.Anything, "b-1").Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/brands/b-1/managers",
			api.AssignManagerRequest{Email: "x@x.com"})
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAssignManager(mock.Anything, "b-1").Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/brands/b-1/managers",
			map[string]any{"email": 42})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("new user returns temp password", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAssignManager(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "mgr@example.com").
			Return(&domain.User{ID: "u-2", Email: "mgr@example.com", Role: api.BrandManager}, "temp-secret-123", nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.AssignManagerResult](t, router, http.MethodPost, "/brands/b-1/managers",
			api.AssignManagerRequest{Email: "mgr@example.com"})

		require.Equal(t, http.StatusCreated, w.Code)
		// Temp password is dynamic (generated) — but here it's the service-level
		// value, so we can assert on it directly.
		require.NotNil(t, resp.Data.TempPassword)
		tempPassword := "temp-secret-123"
		require.Equal(t, api.AssignManagerResult{
			Data: api.AssignManagerData{
				UserId:       "u-2",
				Email:        "mgr@example.com",
				Role:         string(api.BrandManager),
				TempPassword: &tempPassword,
			},
		}, resp)
	})

	t.Run("existing user has no temp password", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAssignManager(mock.Anything, "b-1").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "existing@example.com").
			Return(&domain.User{ID: "u-2", Email: "existing@example.com", Role: api.BrandManager}, "", nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.AssignManagerResult](t, router, http.MethodPost, "/brands/b-1/managers",
			api.AssignManagerRequest{Email: "existing@example.com"})

		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.AssignManagerResult{
			Data: api.AssignManagerData{
				UserId:       "u-2",
				Email:        "existing@example.com",
				Role:         string(api.BrandManager),
				TempPassword: nil,
			},
		}, resp)
	})
}

func TestServer_RemoveManager(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveManager(mock.Anything, "b-1", "u-2").Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, "/brands/b-1/managers/u-2", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("service error returns 404", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveManager(mock.Anything, "b-1", "u-2").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(domain.ErrNotFound)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, "/brands/b-1/managers/u-2", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveManager(mock.Anything, "b-1", "u-2").Return(nil)
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)

		router := newTestRouter(t, NewServer(nil, brands, authz, nil, "test-version", false))
		w, resp := doJSON[api.MessageResponse](t, router, http.MethodDelete, "/brands/b-1/managers/u-2", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.MessageResponse{Data: api.MessageData{Message: "Manager removed"}}, resp)
	})
}
