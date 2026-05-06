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

// CanViewCreators gates the admin POST /creators/list endpoint. The list-item
// shape carries lighter PII than the full GET aggregate (no address, no
// category_other_text, no full Telegram block) but still includes IIN, full
// names, phone and telegram_username, so admin role is the only acceptable
// authorization. Held separately from CanViewCreator so a future widening
// (manager-side catalog browsing on the list, single profile reads still
// admin-only — or vice versa) ships without rippling regressions across the
// other endpoint.
func (a *AuthzService) CanViewCreators(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
