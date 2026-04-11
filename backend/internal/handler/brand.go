package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// Brands is the interface BrandHandler needs from the brand service.
type Brands interface {
	CreateBrand(ctx context.Context, name string, logoURL *string) (repository.BrandRow, error)
	GetBrand(ctx context.Context, id string) (repository.BrandRow, error)
	ListBrands(ctx context.Context, userID, role string) ([]repository.BrandWithManagerCount, error)
	UpdateBrand(ctx context.Context, id, name string, logoURL *string) (repository.BrandRow, error)
	DeleteBrand(ctx context.Context, id string) error
	ListManagers(ctx context.Context, brandID string) ([]repository.BrandManagerRow, error)
	AssignManager(ctx context.Context, brandID, email string) (repository.UserRow, string, error)
	RemoveManager(ctx context.Context, brandID, userID string) error
	CanViewBrand(ctx context.Context, userID, role, brandID string) error
}

// BrandHandler handles brand management endpoints.
type BrandHandler struct {
	brands  Brands
	auditor Auditor
}

// NewBrandHandler creates a new BrandHandler. auditor may be nil.
func NewBrandHandler(brands Brands, auditor Auditor) *BrandHandler {
	return &BrandHandler{brands: brands, auditor: auditor}
}

// CreateBrand handles POST /api/brands
func (h *BrandHandler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	var req struct {
		Name    string  `json:"name"`
		LogoURL *string `json:"logoUrl,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	brand, err := h.brands.CreateBrand(r.Context(), req.Name, req.LogoURL)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), h.auditor, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: role,
		Action: "brand_create", EntityType: "brand", EntityID: brand.ID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusCreated, brandToJSON(brand))
}

// ListBrands handles GET /api/brands
func (h *BrandHandler) ListBrands(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	role := middleware.RoleFromContext(r.Context())

	brands, err := h.brands.ListBrands(r.Context(), userID, role)
	if err != nil {
		respondError(w, r, err)
		return
	}

	items := make([]map[string]any, len(brands))
	for i, b := range brands {
		items[i] = map[string]any{
			"id":           b.ID,
			"name":         b.Name,
			"logoUrl":      b.LogoURL,
			"managerCount": b.ManagerCount,
			"createdAt":    b.CreatedAt,
			"updatedAt":    b.UpdatedAt,
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"brands": items,
	})
}

// GetBrand handles GET /api/brands/{brandID}
func (h *BrandHandler) GetBrand(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	role := middleware.RoleFromContext(r.Context())
	brandID := chi.URLParam(r, "brandID")

	if err := h.brands.CanViewBrand(r.Context(), userID, role, brandID); err != nil {
		respondError(w, r, err)
		return
	}

	brand, err := h.brands.GetBrand(r.Context(), brandID)
	if err != nil {
		respondError(w, r, err)
		return
	}

	managers, err := h.brands.ListManagers(r.Context(), brandID)
	if err != nil {
		respondError(w, r, err)
		return
	}

	managerList := make([]map[string]any, len(managers))
	for i, m := range managers {
		managerList[i] = map[string]any{
			"userId":     m.UserID,
			"email":      m.Email,
			"assignedAt": m.CreatedAt,
		}
	}

	resp := brandToJSON(brand)
	resp["managers"] = managerList

	respondJSON(w, http.StatusOK, resp)
}

// UpdateBrand handles PUT /api/brands/{brandID}
func (h *BrandHandler) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	brandID := chi.URLParam(r, "brandID")

	var req struct {
		Name    string  `json:"name"`
		LogoURL *string `json:"logoUrl,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	brand, err := h.brands.UpdateBrand(r.Context(), brandID, req.Name, req.LogoURL)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), h.auditor, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: role,
		Action: "brand_update", EntityType: "brand", EntityID: brandID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, brandToJSON(brand))
}

// DeleteBrand handles DELETE /api/brands/{brandID}
func (h *BrandHandler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	brandID := chi.URLParam(r, "brandID")

	if err := h.brands.DeleteBrand(r.Context(), brandID); err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), h.auditor, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: role,
		Action: "brand_delete", EntityType: "brand", EntityID: brandID,
		IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Brand deleted",
	})
}

// AssignManager handles POST /api/brands/{brandID}/managers
func (h *BrandHandler) AssignManager(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	brandID := chi.URLParam(r, "brandID")

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	user, tempPassword, err := h.brands.AssignManager(r.Context(), brandID, req.Email)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), h.auditor, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: role,
		Action: "manager_assign", EntityType: "brand", EntityID: brandID,
		NewValue: map[string]string{"email": user.Email}, IPAddress: clientIP(r),
	})

	resp := map[string]any{
		"userId": user.ID,
		"email":  user.Email,
		"role":   user.Role,
	}
	if tempPassword != "" {
		resp["tempPassword"] = tempPassword
	}

	respondJSON(w, http.StatusCreated, resp)
}

// RemoveManager handles DELETE /api/brands/{brandID}/managers/{userID}
func (h *BrandHandler) RemoveManager(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	brandID := chi.URLParam(r, "brandID")
	userID := chi.URLParam(r, "userID")

	if err := h.brands.RemoveManager(r.Context(), brandID, userID); err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), h.auditor, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: role,
		Action: "manager_remove", EntityType: "brand", EntityID: brandID,
		OldValue: map[string]string{"userId": userID}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Manager removed",
	})
}

func brandToJSON(b repository.BrandRow) map[string]any {
	return map[string]any{
		"id":        b.ID,
		"name":      b.Name,
		"logoUrl":   b.LogoURL,
		"createdAt": b.CreatedAt,
		"updatedAt": b.UpdatedAt,
	}
}
