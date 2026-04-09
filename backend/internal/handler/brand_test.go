package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// brandRequest creates an HTTP request with auth context and optional Chi URL params.
func brandRequest(method, body, userID, role string, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, "/", nil)
	}

	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, role)

	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}

	return r.WithContext(ctx)
}

// --- CreateBrand ---

func TestCreateBrandHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().CreateBrand(mock.Anything, "Test Brand", (*string)(nil)).
		Return(repository.BrandRow{ID: "b-1", Name: "Test Brand"}, nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"name":"Test Brand"}`, "u-1", "admin", nil)
	h.CreateBrand(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	resp := parseResponse(t, w)
	var data map[string]any
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	assert.Equal(t, "Test Brand", data["name"])
}

func TestCreateBrandHandler_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"name":"Test"}`, "u-1", "brand_manager", nil)
	h.CreateBrand(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateBrandHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("POST", "not json", "u-1", "admin", nil)
	h.CreateBrand(w, r)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestCreateBrandHandler_ValidationError(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().CreateBrand(mock.Anything, "", (*string)(nil)).
		Return(repository.BrandRow{}, domain.NewValidationError("VALIDATION_ERROR", "Brand name is required"))

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"name":""}`, "u-1", "admin", nil)
	h.CreateBrand(w, r)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- ListBrands ---

func TestListBrandsHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().ListBrands(mock.Anything, "u-1", "admin").
		Return([]repository.BrandWithManagerCount{
			{ID: "b-1", Name: "Brand 1", ManagerCount: 2},
		}, nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("GET", "", "u-1", "admin", nil)
	h.ListBrands(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

// --- GetBrand ---

func TestGetBrandHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().CanViewBrand(mock.Anything, "u-1", "admin", "b-1").Return(nil)
	brands.EXPECT().GetBrand(mock.Anything, "b-1").
		Return(repository.BrandRow{ID: "b-1", Name: "Test"}, nil)
	brands.EXPECT().ListManagers(mock.Anything, "b-1").
		Return([]repository.BrandManagerRow{}, nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("GET", "", "u-1", "admin", map[string]string{"brandID": "b-1"})
	h.GetBrand(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := parseResponse(t, w)
	var data map[string]any
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	assert.Equal(t, "Test", data["name"])
	assert.NotNil(t, data["managers"])
}

func TestGetBrandHandler_Forbidden(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().CanViewBrand(mock.Anything, "u-1", "brand_manager", "b-1").
		Return(domain.ErrForbidden)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("GET", "", "u-1", "brand_manager", map[string]string{"brandID": "b-1"})
	h.GetBrand(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- UpdateBrand ---

func TestUpdateBrandHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().UpdateBrand(mock.Anything, "b-1", "New Name", (*string)(nil)).
		Return(repository.BrandRow{ID: "b-1", Name: "New Name"}, nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("PUT", `{"name":"New Name"}`, "u-1", "admin", map[string]string{"brandID": "b-1"})
	h.UpdateBrand(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateBrandHandler_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("PUT", `{"name":"X"}`, "u-1", "brand_manager", map[string]string{"brandID": "b-1"})
	h.UpdateBrand(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- DeleteBrand ---

func TestDeleteBrandHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().DeleteBrand(mock.Anything, "b-1").Return(nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("DELETE", "", "u-1", "admin", map[string]string{"brandID": "b-1"})
	h.DeleteBrand(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteBrandHandler_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("DELETE", "", "u-1", "brand_manager", map[string]string{"brandID": "b-1"})
	h.DeleteBrand(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- AssignManager ---

func TestAssignManagerHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().AssignManager(mock.Anything, "b-1", "mgr@example.com").
		Return(repository.UserRow{ID: "u-2", Email: "mgr@example.com", Role: "brand_manager"}, "temp123", nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"email":"mgr@example.com"}`, "u-1", "admin", map[string]string{"brandID": "b-1"})
	h.AssignManager(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	resp := parseResponse(t, w)
	var data map[string]any
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	assert.Equal(t, "mgr@example.com", data["email"])
	assert.Equal(t, "temp123", data["tempPassword"])
}

func TestAssignManagerHandler_ExistingUser(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().AssignManager(mock.Anything, "b-1", "existing@example.com").
		Return(repository.UserRow{ID: "u-2", Email: "existing@example.com", Role: "brand_manager"}, "", nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"email":"existing@example.com"}`, "u-1", "admin", map[string]string{"brandID": "b-1"})
	h.AssignManager(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	resp := parseResponse(t, w)
	var data map[string]any
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	assert.Nil(t, data["tempPassword"], "existing user should not get temp password")
}

func TestAssignManagerHandler_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("POST", `{"email":"x@x.com"}`, "u-1", "brand_manager", map[string]string{"brandID": "b-1"})
	h.AssignManager(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- RemoveManager ---

func TestRemoveManagerHandler_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrands(t)
	brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)

	h := NewBrandHandler(brands)
	w := httptest.NewRecorder()
	r := brandRequest("DELETE", "", "u-1", "admin", map[string]string{"brandID": "b-1", "userID": "u-2"})
	h.RemoveManager(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemoveManagerHandler_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	h := NewBrandHandler(nil)
	w := httptest.NewRecorder()
	r := brandRequest("DELETE", "", "u-1", "brand_manager", map[string]string{"brandID": "b-1", "userID": "u-2"})
	h.RemoveManager(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
