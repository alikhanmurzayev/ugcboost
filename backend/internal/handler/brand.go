package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// CreateBrand handles POST /brands
func (s *Server) CreateBrand(w http.ResponseWriter, r *http.Request) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	var req api.CreateBrandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	brand, err := s.brandService.CreateBrand(r.Context(), req.Name, req.LogoUrl)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "brand_create", EntityType: "brand", EntityID: brand.ID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusCreated, api.BrandResult{
		Data: domainBrandToAPI(brand),
	})
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

	items := make([]api.BrandListItem, len(brands))
	for i, b := range brands {
		items[i] = api.BrandListItem{
			Id:           b.ID,
			Name:         b.Name,
			LogoUrl:      b.LogoURL,
			ManagerCount: b.ManagerCount,
			CreatedAt:    b.CreatedAt,
			UpdatedAt:    b.UpdatedAt,
		}
	}

	respondJSON(w, http.StatusOK, api.ListBrandsResult{
		Data: struct {
			Brands []api.BrandListItem `json:"brands"`
		}{Brands: items},
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

	managerList := make([]api.ManagerInfo, len(managers))
	for i, m := range managers {
		managerList[i] = api.ManagerInfo{
			UserId:     m.UserID,
			Email:      m.Email,
			AssignedAt: m.AssignedAt,
		}
	}

	respondJSON(w, http.StatusOK, api.GetBrandResult{
		Data: struct {
			CreatedAt time.Time        `json:"createdAt"`
			Id        string           `json:"id"`
			LogoUrl   *string          `json:"logoUrl,omitempty"`
			Managers  []api.ManagerInfo `json:"managers"`
			Name      string           `json:"name"`
			UpdatedAt time.Time        `json:"updatedAt"`
		}{
			Id:        brand.ID,
			Name:      brand.Name,
			LogoUrl:   brand.LogoURL,
			Managers:  managerList,
			CreatedAt: brand.CreatedAt,
			UpdatedAt: brand.UpdatedAt,
		},
	})
}

// UpdateBrand handles PUT /brands/{brandID}
func (s *Server) UpdateBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	var req api.UpdateBrandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	brand, err := s.brandService.UpdateBrand(r.Context(), brandID, req.Name, req.LogoUrl)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: middleware.UserIDFromContext(r.Context()), ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "brand_update", EntityType: "brand", EntityID: brandID,
		NewValue: map[string]string{"name": brand.Name}, IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, api.BrandResult{
		Data: domainBrandToAPI(brand),
	})
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

	respondJSON(w, http.StatusOK, api.MessageResponse{
		Data: struct {
			Message string `json:"message"`
		}{Message: "Brand deleted"},
	})
}

// AssignManager handles POST /brands/{brandID}/managers
func (s *Server) AssignManager(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	var req api.AssignManagerRequest
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

	var tp *string
	if tempPassword != "" {
		tp = &tempPassword
	}

	respondJSON(w, http.StatusCreated, api.AssignManagerResult{
		Data: struct {
			Email        string  `json:"email"`
			Role         string  `json:"role"`
			TempPassword *string `json:"tempPassword,omitempty"`
			UserId       string  `json:"userId"`
		}{
			UserId:       user.ID,
			Email:        user.Email,
			Role:         string(user.Role),
			TempPassword: tp,
		},
	})
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

	respondJSON(w, http.StatusOK, api.MessageResponse{
		Data: struct {
			Message string `json:"message"`
		}{Message: "Manager removed"},
	})
}

func domainBrandToAPI(b *domain.Brand) api.Brand {
	return api.Brand{
		Id:        b.ID,
		Name:      b.Name,
		LogoUrl:   b.LogoURL,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}
