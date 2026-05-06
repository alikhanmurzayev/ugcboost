package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestAuthzService_CanCreateCampaign(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanCreateCampaign(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanCreateCampaign(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanCreateCampaign(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanGetCampaign(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanGetCampaign(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanGetCampaign(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanGetCampaign(ctxWithRole(api.Admin)))
	})
}
