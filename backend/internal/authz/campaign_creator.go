package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanAddCampaignCreators gates POST /campaigns/{id}/creators to admins. Brand
// managers and any future non-admin role receive domain.ErrForbidden — the
// composition of a campaign roster is an admin-curated decision in the
// current MVP (see campaign-roadmap.md chunk 10).
func (a *AuthzService) CanAddCampaignCreators(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanRemoveCampaignCreator gates DELETE /campaigns/{id}/creators/{creatorId}
// to admins only.
func (a *AuthzService) CanRemoveCampaignCreator(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanListCampaignCreators gates GET /campaigns/{id}/creators to admins only.
func (a *AuthzService) CanListCampaignCreators(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
