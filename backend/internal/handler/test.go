package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// TestSeeder creates users for testing.
type TestSeeder interface {
	SeedUser(ctx context.Context, email, password, role string) (repository.UserRow, error)
}

// TokenStore retrieves raw reset tokens captured in memory.
type TokenStore interface {
	GetToken(email string) (string, bool)
}

// TestHandler provides test-only endpoints.
// Only registered when ENABLE_TEST_ENDPOINTS=true.
type TestHandler struct {
	seeder     TestSeeder
	tokenStore TokenStore
}

// NewTestHandler creates a new TestHandler.
func NewTestHandler(seeder TestSeeder, tokenStore TokenStore) *TestHandler {
	return &TestHandler{seeder: seeder, tokenStore: tokenStore}
}

// SeedUser handles POST /test/seed-user
func (h *TestHandler) SeedUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Invalid request body"))
		return
	}

	if req.Email == "" || req.Password == "" || req.Role == "" {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "email, password, and role are required"))
		return
	}

	user, err := h.seeder.SeedUser(r.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		respondError(w, r, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
	})
}

// GetResetToken handles GET /test/reset-tokens?email=...
func (h *TestHandler) GetResetToken(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "email query parameter is required"))
		return
	}

	token, ok := h.tokenStore.GetToken(email)
	if !ok {
		respondError(w, r, domain.ErrNotFound)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"token": token,
	})
}
