package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// CanApproveCreator checks that the user is an admin.
func CanApproveCreator(_ context.Context, _ dbutil.DB, role string) error {
	if role != string(domain.RoleAdmin) {
		return domain.ErrForbidden
	}
	return nil
}

// CanManageCampaign checks that the user is a manager of the brand that owns the campaign.
// Actual DB check will be implemented when campaigns exist.
func CanManageCampaign(_ context.Context, _ dbutil.DB, _ string, _ string) error {
	// TODO: implement when campaigns table exists
	return nil
}

// CanManageBrand checks that the user is an admin or a manager of the specified brand.
func CanManageBrand(ctx context.Context, db dbutil.DB, userID string, role string, brandID string) error {
	if role == string(domain.RoleAdmin) {
		return nil
	}

	q := dbutil.Psql.Select("1").
		From(repository.TableBrandManagers).
		Where(repository.BrandManagerColumnUserID+" = ? AND "+repository.BrandManagerColumnBrandID+" = ?", userID, brandID).
		Limit(1)

	_, err := dbutil.Val[int](ctx, db, q)
	if err != nil {
		return domain.ErrForbidden
	}
	return nil
}
