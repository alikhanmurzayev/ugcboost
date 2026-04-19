package handler

import (
	"encoding/json"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// CreateBrand handles POST /brands
func (s *Server) CreateBrand(w http.ResponseWriter, r *http.Request) {
	if err := s.authzService.CanCreateBrand(r.Context()); err != nil {
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

	respondJSON(w, r, http.StatusCreated, api.BrandResult{
		Data: domainBrandToAPI(brand),
	})
}

// ListBrands handles GET /brands
func (s *Server) ListBrands(w http.ResponseWriter, r *http.Request) {
	canViewAll, userID, err := s.authzService.CanListBrands(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}

	var managerID *string
	if !canViewAll {
		managerID = &userID
	}

	brands, err := s.brandService.ListBrands(r.Context(), managerID)
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

	respondJSON(w, r, http.StatusOK, api.ListBrandsResult{
		Data: api.ListBrandsData{Brands: items},
	})
}

// GetBrand handles GET /brands/{brandID}
func (s *Server) GetBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := s.authzService.CanViewBrand(r.Context(), brandID); err != nil {
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

	respondJSON(w, r, http.StatusOK, api.GetBrandResult{
		Data: api.BrandDetailData{
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
	if err := s.authzService.CanUpdateBrand(r.Context(), brandID); err != nil {
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

	respondJSON(w, r, http.StatusOK, api.BrandResult{
		Data: domainBrandToAPI(brand),
	})
}

// DeleteBrand handles DELETE /brands/{brandID}
func (s *Server) DeleteBrand(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := s.authzService.CanDeleteBrand(r.Context(), brandID); err != nil {
		respondError(w, r, err)
		return
	}

	if err := s.brandService.DeleteBrand(r.Context(), brandID); err != nil {
		respondError(w, r, err)
		return
	}

	respondJSON(w, r, http.StatusOK, api.MessageResponse{
		Data: api.MessageData{Message: "Brand deleted"},
	})
}

// AssignManager handles POST /brands/{brandID}/managers
func (s *Server) AssignManager(w http.ResponseWriter, r *http.Request, brandID string) {
	if err := s.authzService.CanAssignManager(r.Context(), brandID); err != nil {
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

	var tp *string
	if tempPassword != "" {
		tp = &tempPassword
	}

	respondJSON(w, r, http.StatusCreated, api.AssignManagerResult{
		Data: api.AssignManagerData{
			UserId:       user.ID,
			Email:        user.Email,
			Role:         string(user.Role),
			TempPassword: tp,
		},
	})
}

// RemoveManager handles DELETE /brands/{brandID}/managers/{userID}
func (s *Server) RemoveManager(w http.ResponseWriter, r *http.Request, brandID string, userID string) {
	if err := s.authzService.CanRemoveManager(r.Context(), brandID, userID); err != nil {
		respondError(w, r, err)
		return
	}

	if err := s.brandService.RemoveManager(r.Context(), brandID, userID); err != nil {
		respondError(w, r, err)
		return
	}

	respondJSON(w, r, http.StatusOK, api.MessageResponse{
		Data: api.MessageData{Message: "Manager removed"},
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
