package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanViewCreatorApplication allows only admins to read a creator application
// aggregate. Brand managers (and any future non-admin role) get
// domain.ErrForbidden — the endpoint exposes raw PII, so the rule stays
// strict admin-only regardless of brand membership.
func (a *AuthzService) CanViewCreatorApplication(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanListCreatorApplications gates the admin moderation list endpoint
// (POST /creators/applications/list). The list-item shape carries lighter
// PII than the full GET aggregate (no phone/address/consents) but still
// includes IIN and full names, so admin role is the only acceptable
// authorization. Returning the same domain.ErrForbidden as the GET keeps
// the 403 message and timing identical between the two endpoints.
func (a *AuthzService) CanListCreatorApplications(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
