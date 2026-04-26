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
