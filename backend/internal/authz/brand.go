package authz

import (
	"context"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanCreateBrand allows only admins to create brands.
func (a *AuthzService) CanCreateBrand(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}

// CanListBrands reports whether the caller may see every brand or only ones they manage.
// canViewAll = true for admins; for brand managers, userID is returned to restrict the list.
func (a *AuthzService) CanListBrands(ctx context.Context) (canViewAll bool, userID string, err error) {
	uid := middleware.UserIDFromContext(ctx)
	if middleware.RoleFromContext(ctx) == api.Admin {
		return true, uid, nil
	}
	return false, uid, nil
}

// CanViewBrand allows admins and managers of the specified brand.
func (a *AuthzService) CanViewBrand(ctx context.Context, brandID string) error {
	if middleware.RoleFromContext(ctx) == api.Admin {
		return nil
	}
	ok, err := a.brandService.IsUserBrandManager(ctx, middleware.UserIDFromContext(ctx), brandID)
	if err != nil {
		return fmt.Errorf("check brand access: %w", err)
	}
	if !ok {
		return domain.ErrForbidden
	}
	return nil
}

// CanUpdateBrand allows only admins.
func (a *AuthzService) CanUpdateBrand(ctx context.Context, _ string) error {
	return a.CanCreateBrand(ctx)
}

// CanDeleteBrand allows only admins.
func (a *AuthzService) CanDeleteBrand(ctx context.Context, _ string) error {
	return a.CanCreateBrand(ctx)
}

// CanAssignManager allows only admins.
func (a *AuthzService) CanAssignManager(ctx context.Context, _ string) error {
	return a.CanCreateBrand(ctx)
}

// CanRemoveManager allows only admins.
func (a *AuthzService) CanRemoveManager(ctx context.Context, _ string, _ string) error {
	return a.CanCreateBrand(ctx)
}
