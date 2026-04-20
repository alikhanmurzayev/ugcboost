package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
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

// TestAPICleanupRepoFactory is the narrow repo-factory interface the test
// cleanup endpoint uses. Kept separate from the production repo factory so
// the test handler only sees the constructors it actually needs.
type TestAPICleanupRepoFactory interface {
	NewUserRepo(db dbutil.DB) repository.UserRepo
	NewBrandRepo(db dbutil.DB) repository.BrandRepo
}

// TestAPIHandler provides test-only endpoints that back openapi-test.yaml.
// Only registered when ENVIRONMENT != production. The cleanup endpoint
// reaches into the repository layer directly (rather than through a service)
// because the hard-delete semantics are test-only and must not leak into
// production call sites — see repository.UserRepo.DeleteForTests for details.
type TestAPIHandler struct {
	auth       TestAPIAuthService
	pool       dbutil.Pool
	repos      TestAPICleanupRepoFactory
	tokenStore TokenStore
	logger     logger.Logger
}

var _ testapi.ServerInterface = (*TestAPIHandler)(nil)

// NewTestAPIHandler creates a new TestAPIHandler.
func NewTestAPIHandler(
	auth TestAPIAuthService,
	pool dbutil.Pool,
	repos TestAPICleanupRepoFactory,
	tokenStore TokenStore,
	log logger.Logger,
) *TestAPIHandler {
	return &TestAPIHandler{
		auth:       auth,
		pool:       pool,
		repos:      repos,
		tokenStore: tokenStore,
		logger:     log,
	}
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

// CleanupEntity handles POST /test/cleanup-entity.
// Dispatches by req.Type: "user" hard-deletes the user and its references
// inside a transaction; "brand" forwards to the standard brand delete.
func (h *TestAPIHandler) CleanupEntity(w http.ResponseWriter, r *http.Request) {
	var req testapi.CleanupEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}

	if req.Id == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "id is required"), h.logger)
		return
	}

	var deleteErr error
	switch req.Type {
	case testapi.User:
		deleteErr = dbutil.WithTx(r.Context(), h.pool, func(tx dbutil.DB) error {
			return h.repos.NewUserRepo(tx).DeleteForTests(r.Context(), req.Id)
		})
	case testapi.Brand:
		deleteErr = h.repos.NewBrandRepo(h.pool).Delete(r.Context(), req.Id)
	default:
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "type must be 'user' or 'brand'"), h.logger)
		return
	}

	if deleteErr != nil {
		if errors.Is(deleteErr, sql.ErrNoRows) {
			respondError(w, r, domain.ErrNotFound, h.logger)
			return
		}
		respondError(w, r, deleteErr, h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
