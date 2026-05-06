package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanCreateCampaign gates POST /campaigns to admins only. Brand managers and
// any future non-admin role receive domain.ErrForbidden — campaigns are an
// admin-curated catalog in the current MVP, brand-self-service is explicitly
// out of scope for this phase (see campaign-roadmap.md).
func (a *AuthzService) CanCreateCampaign(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanGetCampaign gates GET /campaigns/{id} to admins only.
func (a *AuthzService) CanGetCampaign(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
