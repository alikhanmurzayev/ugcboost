package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// BrandAccessChecker checks if a user manages a specific brand.
type BrandAccessChecker interface {
	IsManager(ctx context.Context, userID, brandID string) (bool, error)
}

// RequireAdmin checks that the authenticated user has admin role.
func RequireAdmin(ctx context.Context) error {
	role := middleware.RoleFromContext(ctx)
	if role != string(domain.RoleAdmin) {
		return domain.ErrForbidden
	}
	return nil
}

// CanApproveCreator checks that the user is an admin.
func CanApproveCreator(role string) error {
	if role != string(domain.RoleAdmin) {
		return domain.ErrForbidden
	}
	return nil
}

// CanManageCampaign checks that the user is a manager of the brand that owns the campaign.
// Actual DB check will be implemented when campaigns exist.
func CanManageCampaign() error {
	// TODO(#16): implement when campaigns table exists
	return nil
}

// CanManageBrand checks that the user is an admin or a manager of the specified brand.
func CanManageBrand(ctx context.Context, checker BrandAccessChecker, userID string, role string, brandID string) error {
	if role == string(domain.RoleAdmin) {
		return nil
	}

	ok, err := checker.IsManager(ctx, userID, brandID)
	if err != nil || !ok {
		return domain.ErrForbidden
	}
	return nil
}
