package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// CreateBrand handles POST /brands.
func (s *Server) CreateBrand(ctx context.Context, request api.CreateBrandRequestObject) (api.CreateBrandResponseObject, error) {
	if err := s.authzService.CanCreateBrand(ctx); err != nil {
		return nil, err
	}

	brand, err := s.brandService.CreateBrand(ctx, request.Body.Name, request.Body.LogoUrl)
	if err != nil {
		return nil, err
	}

	return api.CreateBrand201JSONResponse{
		Data: domainBrandToAPI(brand),
	}, nil
}

// ListBrands handles GET /brands.
func (s *Server) ListBrands(ctx context.Context, _ api.ListBrandsRequestObject) (api.ListBrandsResponseObject, error) {
	canViewAll, userID, err := s.authzService.CanListBrands(ctx)
	if err != nil {
		return nil, err
	}

	var managerID *string
	if !canViewAll {
		managerID = &userID
	}

	brands, err := s.brandService.ListBrands(ctx, managerID)
	if err != nil {
		return nil, err
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

	return api.ListBrands200JSONResponse{
		Data: api.ListBrandsData{Brands: items},
	}, nil
}

// GetBrand handles GET /brands/{brandID}.
func (s *Server) GetBrand(ctx context.Context, request api.GetBrandRequestObject) (api.GetBrandResponseObject, error) {
	if err := s.authzService.CanViewBrand(ctx, request.BrandID); err != nil {
		return nil, err
	}

	brand, err := s.brandService.GetBrand(ctx, request.BrandID)
	if err != nil {
		return nil, err
	}

	managers, err := s.brandService.ListManagers(ctx, request.BrandID)
	if err != nil {
		return nil, err
	}

	managerList := make([]api.ManagerInfo, len(managers))
	for i, m := range managers {
		managerList[i] = api.ManagerInfo{
			UserId:     m.UserID,
			Email:      m.Email,
			AssignedAt: m.AssignedAt,
		}
	}

	return api.GetBrand200JSONResponse{
		Data: api.BrandDetailData{
			Id:        brand.ID,
			Name:      brand.Name,
			LogoUrl:   brand.LogoURL,
			Managers:  managerList,
			CreatedAt: brand.CreatedAt,
			UpdatedAt: brand.UpdatedAt,
		},
	}, nil
}

// UpdateBrand handles PUT /brands/{brandID}.
func (s *Server) UpdateBrand(ctx context.Context, request api.UpdateBrandRequestObject) (api.UpdateBrandResponseObject, error) {
	if err := s.authzService.CanUpdateBrand(ctx, request.BrandID); err != nil {
		return nil, err
	}

	brand, err := s.brandService.UpdateBrand(ctx, request.BrandID, request.Body.Name, request.Body.LogoUrl)
	if err != nil {
		return nil, err
	}

	return api.UpdateBrand200JSONResponse{
		Data: domainBrandToAPI(brand),
	}, nil
}

// DeleteBrand handles DELETE /brands/{brandID}.
func (s *Server) DeleteBrand(ctx context.Context, request api.DeleteBrandRequestObject) (api.DeleteBrandResponseObject, error) {
	if err := s.authzService.CanDeleteBrand(ctx, request.BrandID); err != nil {
		return nil, err
	}

	if err := s.brandService.DeleteBrand(ctx, request.BrandID); err != nil {
		return nil, err
	}

	return api.DeleteBrand200JSONResponse{
		Data: api.MessageData{Message: "Brand deleted"},
	}, nil
}

// AssignManager handles POST /brands/{brandID}/managers.
func (s *Server) AssignManager(ctx context.Context, request api.AssignManagerRequestObject) (api.AssignManagerResponseObject, error) {
	if err := s.authzService.CanAssignManager(ctx, request.BrandID); err != nil {
		return nil, err
	}

	user, tempPassword, err := s.brandService.AssignManager(ctx, request.BrandID, request.Body.Email)
	if err != nil {
		return nil, err
	}

	var tp *string
	if tempPassword != "" {
		tp = &tempPassword
	}

	return api.AssignManager201JSONResponse{
		Data: api.AssignManagerData{
			UserId:       user.ID,
			Email:        user.Email,
			Role:         string(user.Role),
			TempPassword: tp,
		},
	}, nil
}

// RemoveManager handles DELETE /brands/{brandID}/managers/{userID}.
func (s *Server) RemoveManager(ctx context.Context, request api.RemoveManagerRequestObject) (api.RemoveManagerResponseObject, error) {
	if err := s.authzService.CanRemoveManager(ctx, request.BrandID, request.UserID); err != nil {
		return nil, err
	}

	if err := s.brandService.RemoveManager(ctx, request.BrandID, request.UserID); err != nil {
		return nil, err
	}

	return api.RemoveManager200JSONResponse{
		Data: api.MessageData{Message: "Manager removed"},
	}, nil
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
