package authz

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// CanReadTelegramMessages gates GET /telegram-messages. The endpoint returns
// raw inbound user text, so admin role is the only acceptable authorization.
// Same domain.ErrForbidden shape as the other admin-only read endpoints keeps
// 403 message and timing uniform.
func (a *AuthzService) CanReadTelegramMessages(ctx context.Context) error {
	if middleware.RoleFromContext(ctx) != api.Admin {
		return domain.ErrForbidden
	}
	return nil
}
