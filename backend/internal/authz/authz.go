package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// BrandService defines what AuthzService needs from the brand service.
type BrandService interface {
	IsUserBrandManager(ctx context.Context, userID, brandID string) (bool, error)
}

// RepoFactory creates the read-only repos AuthorizeTMACampaignDecision
// uses. AuthzService keeps the lookup tx-less because the call sits in
// front of the business service (which opens its own tx with FOR UPDATE)
// — this read just resolves resource access, the row state-machine guard
// lives downstream.
type RepoFactory interface {
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
}

// AuthzService centralises authorisation decisions for API actions.
// Each method inspects the request context (user ID, role) and
// returns an error when the caller is not allowed to perform the action.
type AuthzService struct {
	brandService BrandService
	pool         dbutil.Pool
	repoFactory  RepoFactory
}

// NewAuthzService creates a new AuthzService.
func NewAuthzService(brandService BrandService, pool dbutil.Pool, repoFactory RepoFactory) *AuthzService {
	return &AuthzService{brandService: brandService, pool: pool, repoFactory: repoFactory}
}
