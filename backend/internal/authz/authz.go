package authz

import (
	"context"
)

// BrandService defines what AuthzService needs from the brand service.
type BrandService interface {
	IsUserBrandManager(ctx context.Context, userID, brandID string) (bool, error)
}

// AuthzService centralises authorisation decisions for API actions.
// Each method inspects the request context (user ID, role) and
// returns an error when the caller is not allowed to perform the action.
type AuthzService struct {
	brandService BrandService
}

// NewAuthzService creates a new AuthzService.
func NewAuthzService(brandService BrandService) *AuthzService {
	return &AuthzService{brandService: brandService}
}
