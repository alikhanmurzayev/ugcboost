package handler

import (
	"encoding/json"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// TokenStore retrieves raw reset tokens captured in memory.
type TokenStore interface {
	GetToken(email string) (string, bool)
}

// TestHandler provides test-only endpoints.
// Only registered when ENVIRONMENT=local.
type TestHandler struct {
	auth       AuthService
	brands     BrandService
	tokenStore TokenStore
}

// NewTestHandler creates a new TestHandler.
func NewTestHandler(auth AuthService, brands BrandService, tokenStore TokenStore) *TestHandler {
	return &TestHandler{auth: auth, brands: brands, tokenStore: tokenStore}
}

// SeedUser handles POST /test/seed-user
func (h *TestHandler) SeedUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	if req.Email == "" || req.Password == "" || req.Role == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "email, password, and role are required"))
		return
	}

	user, err := h.auth.SeedUser(r.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		respondError(w, r, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}

// SeedBrand handles POST /test/seed-brand
func (h *TestHandler) SeedBrand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		ManagerEmail string `json:"managerEmail,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	if req.Name == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "name is required"))
		return
	}

	brand, err := h.brands.CreateBrand(r.Context(), req.Name, nil)
	if err != nil {
		respondError(w, r, err)
		return
	}

	if req.ManagerEmail != "" {
		if _, _, err := h.brands.AssignManager(r.Context(), brand.ID, req.ManagerEmail); err != nil {
			respondError(w, r, err)
			return
		}
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":   brand.ID,
			"name": brand.Name,
		},
	})
}

// GetResetToken handles GET /test/reset-tokens?email=...
func (h *TestHandler) GetResetToken(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "email query parameter is required"))
		return
	}

	token, ok := h.tokenStore.GetToken(email)
	if !ok {
		respondError(w, r, domain.ErrNotFound)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"token": token,
		},
	})
}
