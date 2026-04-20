package authz

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestAuthzService_CanListAuditLogs(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanListAuditLogs(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanListAuditLogs(ctxWithRole(api.Admin)))
	})
}
