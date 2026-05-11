package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestAuthzService_CanAddCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanAddCampaignCreators(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanAddCampaignCreators(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanAddCampaignCreators(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanRemoveCampaignCreator(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemoveCampaignCreator(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemoveCampaignCreator(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanRemoveCampaignCreator(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanListCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanListCampaignCreators(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanListCampaignCreators(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanListCampaignCreators(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanNotifyCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanNotifyCampaignCreators(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanNotifyCampaignCreators(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanNotifyCampaignCreators(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanRemindCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemindCampaignCreators(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemindCampaignCreators(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanRemindCampaignCreators(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanRemindCampaignCreatorsSigning(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemindCampaignCreatorsSigning(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.ErrorIs(t, svc.CanRemindCampaignCreatorsSigning(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanRemindCampaignCreatorsSigning(ctxWithRole(api.Admin)))
	})
}
