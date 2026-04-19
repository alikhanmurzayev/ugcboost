package handler

import (
	"context"
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// TokenStore retrieves raw reset tokens captured in memory.
type TokenStore interface {
	GetToken(email string) (string, bool)
}

// TestAPIAuthService is the subset of auth operations the test handler needs.
// Kept separate from AuthService so production callers never see SeedUser.
type TestAPIAuthService interface {
	SeedUser(ctx context.Context, email, password, role string) (*domain.User, error)
}

// TestAPIBrandService is the subset of prod BrandService methods the test
// endpoints need. Kept separate so the generic BrandService interface stays
// minimal and the test handler only depends on the operations it actually
// uses.
type TestAPIBrandService interface {
	CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error)
	AssignManager(ctx context.Context, brandID, email string) (*domain.User, string, error)
}

// TestAPIHandler provides test-only endpoints that back openapi-test.yaml.
// Only registered when ENVIRONMENT=local. Seed-brand operations need a real
// actor for audit FK integrity, so the handler impersonates the seed admin
// when calling the production brand service.
type TestAPIHandler struct {
	auth       TestAPIAuthService
	brands     TestAPIBrandService
	tokenStore TokenStore
	adminID    string
	logger     logger.Logger
}

var _ testapi.ServerInterface = (*TestAPIHandler)(nil)

// NewTestAPIHandler creates a new TestAPIHandler. adminID must be the seed
// admin's user ID (resolved at startup from cfg.AdminEmail); it is written
// into the request context for brand-seed operations so audit rows have a
// valid actor.
func NewTestAPIHandler(auth TestAPIAuthService, brands TestAPIBrandService, tokenStore TokenStore, adminID string, log logger.Logger) *TestAPIHandler {
	return &TestAPIHandler{auth: auth, brands: brands, tokenStore: tokenStore, adminID: adminID, logger: log}
}

// SeedUser handles POST /test/seed-user.
func (h *TestAPIHandler) SeedUser(w http.ResponseWriter, r *http.Request) {
	var req testapi.SeedUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}

	email := string(req.Email)
	if email == "" || req.Password == "" || req.Role == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "email, password, and role are required"), h.logger)
		return
	}

	user, err := h.auth.SeedUser(r.Context(), email, req.Password, string(req.Role))
	if err != nil {
		respondError(w, r, err, h.logger)
		return
	}

	respondJSON(w, r, http.StatusCreated, testapi.SeedUserResult{
		Data: testapi.SeedUserData{
			Id:    user.ID,
			Email: openapi_types.Email(user.Email),
			Role:  testapi.SeedUserDataRole(user.Role),
		},
	}, h.logger)
}

// SeedBrand handles POST /test/seed-brand.
func (h *TestAPIHandler) SeedBrand(w http.ResponseWriter, r *http.Request) {
	var req testapi.SeedBrandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}

	if req.Name == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "name is required"), h.logger)
		return
	}

	// Impersonate the seed admin so audit rows created inside BrandService have
	// a valid actor. The /test endpoints run unauthenticated by design, so the
	// normal auth middleware never populates user/role in the context.
	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, h.adminID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, api.Admin)

	brand, err := h.brands.CreateBrand(ctx, req.Name, nil)
	if err != nil {
		respondError(w, r, err, h.logger)
		return
	}

	if req.ManagerEmail != nil && string(*req.ManagerEmail) != "" {
		if _, _, err := h.brands.AssignManager(ctx, brand.ID, string(*req.ManagerEmail)); err != nil {
			respondError(w, r, err, h.logger)
			return
		}
	}

	respondJSON(w, r, http.StatusCreated, testapi.SeedBrandResult{
		Data: testapi.SeedBrandData{
			Id:   brand.ID,
			Name: brand.Name,
		},
	}, h.logger)
}

// GetResetToken handles GET /test/reset-tokens?email=...
func (h *TestAPIHandler) GetResetToken(w http.ResponseWriter, r *http.Request, params testapi.GetResetTokenParams) {
	email := string(params.Email)
	token, ok := h.tokenStore.GetToken(email)
	if !ok {
		respondError(w, r, domain.ErrNotFound, h.logger)
		return
	}

	respondJSON(w, r, http.StatusOK, testapi.ResetTokenResult{
		Data: testapi.ResetTokenData{Token: token},
	}, h.logger)
}
