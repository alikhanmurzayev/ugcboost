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

// CanUpdateCampaign gates PATCH /campaigns/{id} to admins only.
func (a *AuthzService) CanUpdateCampaign(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanListCampaigns gates GET /campaigns to admins only.
func (a *AuthzService) CanListCampaigns(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanUploadCampaignContractTemplate gates PUT /campaigns/{id}/contract-template
// to admins only — the template is admin-curated content (Аидана), brand
// managers do not see this section in the UI either.
func (a *AuthzService) CanUploadCampaignContractTemplate(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanGetCampaignContractTemplate gates GET /campaigns/{id}/contract-template
// to admins only — the PDF body is internal admin-only content.
func (a *AuthzService) CanGetCampaignContractTemplate(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
