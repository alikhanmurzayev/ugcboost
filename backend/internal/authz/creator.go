package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanViewCreator gates the admin GET /creators/{id} aggregate read.
// The aggregate exposes raw creator PII (IIN, full name, phone, address,
// Telegram metadata, social handles) and per-social verification metadata, so
// the rule stays strict admin-only — brand managers and any future non-admin
// role get domain.ErrForbidden, identical to CanViewCreatorApplication. A
// future widening for brand_manager (campaign-side catalog browsing) ships as
// a separate authz method to keep the timing and message of this 403 stable.
func (a *AuthzService) CanViewCreator(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
