package handler

import (
	"encoding/json"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// CreateBrand handles POST /brands
func (s *Server) CreateBrand(w http.ResponseWriter, r *http.Request) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
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

	brand, err := s.brandService.CreateBrand(r.Context(), req.Name, req.LogoURL)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "brand_create", EntityType: "brand", EntityID: brand.ID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusCreated, brandToJSON(*brand))
}

// ListBrands handles GET /brands
func (s *Server) ListBrands(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	role := middleware.RoleFromContext(r.Context())

	brands, err := s.brandService.ListBrands(r.Context(), userID, role)
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

// GetBrand handles GET /brands/{brandID}
func (s *Server) GetBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	userID := middleware.UserIDFromContext(r.Context())
	role := middleware.RoleFromContext(r.Context())

	if err := s.brandService.CanViewBrand(r.Context(), userID, role, brandID); err != nil {
		respondError(w, r, err)
		return
	}

	brand, err := s.brandService.GetBrand(r.Context(), brandID)
	if err != nil {
		respondError(w, r, err)
		return
	}

	managers, err := s.brandService.ListManagers(r.Context(), brandID)
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

	resp := brandToJSON(*brand)
	resp["managers"] = managerList

	respondJSON(w, http.StatusOK, resp)
}

// UpdateBrand handles PUT /brands/{brandID}
func (s *Server) UpdateBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
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

	brand, err := s.brandService.UpdateBrand(r.Context(), brandID, req.Name, req.LogoURL)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "brand_update", EntityType: "brand", EntityID: brandID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, brandToJSON(*brand))
}

// DeleteBrand handles DELETE /brands/{brandID}
func (s *Server) DeleteBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	if err := s.brandService.DeleteBrand(r.Context(), brandID); err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "brand_delete", EntityType: "brand", EntityID: brandID,
		IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Brand deleted",
	})
}

// AssignManager handles POST /brands/{brandID}/managers
func (s *Server) AssignManager(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	user, tempPassword, err := s.brandService.AssignManager(r.Context(), brandID, req.Email)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
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

// RemoveManager handles DELETE /brands/{brandID}/managers/{userID}
func (s *Server) RemoveManager(w http.ResponseWriter, r *http.Request, brandID string, userID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	if err := s.brandService.RemoveManager(r.Context(), brandID, userID); err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
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
