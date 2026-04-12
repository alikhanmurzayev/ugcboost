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
)

// adminContext creates a request context with admin user credentials.
func adminContext(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, "u-1")
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, "admin")
	return r.WithContext(ctx)
}

// managerContext creates a request context with brand_manager user credentials.
func managerContext(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, "u-1")
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, "brand_manager")
	return r.WithContext(ctx)
}

func TestServer_CreateBrand(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
			Return(&repository.BrandRow{ID: "b-1", Name: "Test Brand"}, nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Test Brand"}`))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.CreateBrand(w, r)
		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Test"}`))
		r.Header.Set("Content-Type", "application/json")
		r = managerContext(r)
		s.CreateBrand(w, r)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader("not json"))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.CreateBrand(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().CreateBrand(mock.Anything, "", (*string)(nil)).
			Return((*repository.BrandRow)(nil), domain.NewValidationError("VALIDATION_ERROR", "Brand name is required"))

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":""}`))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.CreateBrand(w, r)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})
}

func TestServer_ListBrands(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().ListBrands(mock.Anything, "u-1", "admin").
			Return([]*repository.BrandWithManagerCount{
				{ID: "b-1", Name: "Brand 1", ManagerCount: 2},
			}, nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r = adminContext(r)
		s.ListBrands(w, r)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestServer_GetBrand(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().CanViewBrand(mock.Anything, "u-1", "admin", "b-1").Return(nil)
		brands.EXPECT().GetBrand(mock.Anything, "b-1").
			Return(&repository.BrandRow{ID: "b-1", Name: "Test"}, nil)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return([]*repository.BrandManagerRow{}, nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r = adminContext(r)
		s.GetBrand(w, r, "b-1")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().CanViewBrand(mock.Anything, "u-1", "brand_manager", "b-1").
			Return(domain.ErrForbidden)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r = managerContext(r)
		s.GetBrand(w, r, "b-1")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestServer_UpdateBrand(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().UpdateBrand(mock.Anything, "b-1", "New Name", (*string)(nil)).
			Return(&repository.BrandRow{ID: "b-1", Name: "New Name"}, nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/", strings.NewReader(`{"name":"New Name"}`))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.UpdateBrand(w, r, "b-1")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/", strings.NewReader(`{"name":"X"}`))
		r.Header.Set("Content-Type", "application/json")
		r = managerContext(r)
		s.UpdateBrand(w, r, "b-1")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestServer_DeleteBrand(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().DeleteBrand(mock.Anything, "b-1").Return(nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/", nil)
		r = adminContext(r)
		s.DeleteBrand(w, r, "b-1")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/", nil)
		r = managerContext(r)
		s.DeleteBrand(w, r, "b-1")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestServer_AssignManager(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "mgr@example.com").
			Return(&repository.UserRow{ID: "u-2", Email: "mgr@example.com", Role: "brand_manager"}, "temp123", nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"mgr@example.com"}`))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.AssignManager(w, r, "b-1")
		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("existing user", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "existing@example.com").
			Return(&repository.UserRow{ID: "u-2", Email: "existing@example.com", Role: "brand_manager"}, "", nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"existing@example.com"}`))
		r.Header.Set("Content-Type", "application/json")
		r = adminContext(r)
		s.AssignManager(w, r, "b-1")
		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"x@x.com"}`))
		r.Header.Set("Content-Type", "application/json")
		r = managerContext(r)
		s.AssignManager(w, r, "b-1")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestServer_RemoveManager(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		brands := mocks.NewMockBrandService(t)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)

		s := NewServer(nil, brands, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/", nil)
		r = adminContext(r)
		s.RemoveManager(w, r, "b-1", "u-2")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		s := NewServer(nil, nil, nil, false)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/", nil)
		r = managerContext(r)
		s.RemoveManager(w, r, "b-1", "u-2")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}
