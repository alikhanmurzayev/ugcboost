package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// ctxWithRole returns a context carrying the supplied role (and a stable user id).
func ctxWithRole(role api.UserRole) context.Context {
	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserID, "u-1")
	return context.WithValue(ctx, middleware.ContextKeyRole, role)
}

func TestAuthzService_CanCreateBrand(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		err := svc.CanCreateBrand(ctxWithRole(api.BrandManager))
		require.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		err := svc.CanCreateBrand(ctxWithRole(api.Admin))
		require.NoError(t, err)
	})
}

func TestAuthzService_CanListBrands(t *testing.T) {
	t.Parallel()

	t.Run("admin sees all", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		canViewAll, userID, err := svc.CanListBrands(ctxWithRole(api.Admin))
		require.NoError(t, err)
		require.True(t, canViewAll)
		require.Equal(t, "u-1", userID)
	})

	t.Run("manager restricted", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		canViewAll, userID, err := svc.CanListBrands(ctxWithRole(api.BrandManager))
		require.NoError(t, err)
		require.False(t, canViewAll)
		require.Equal(t, "u-1", userID)
	})
}

func TestAuthzService_CanViewBrand(t *testing.T) {
	t.Parallel()

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
		require.NoError(t, svc.CanViewBrand(ctxWithRole(api.Admin), "b-1"))
	})

	t.Run("service error wraps", func(t *testing.T) {
		t.Parallel()
		bs := mocks.NewMockBrandService(t)
		boom := errors.New("boom")
		bs.EXPECT().IsUserBrandManager(mock.Anything, "u-1", "b-1").Return(false, boom)

		svc := NewAuthzService(bs, nil, nil)
		err := svc.CanViewBrand(ctxWithRole(api.BrandManager), "b-1")
		require.ErrorContains(t, err, "check brand access")
		require.ErrorIs(t, err, boom)
	})

	t.Run("not manager forbidden", func(t *testing.T) {
		t.Parallel()
		bs := mocks.NewMockBrandService(t)
		bs.EXPECT().IsUserBrandManager(mock.Anything, "u-1", "b-1").Return(false, nil)

		svc := NewAuthzService(bs, nil, nil)
		err := svc.CanViewBrand(ctxWithRole(api.BrandManager), "b-1")
		require.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("manager allowed", func(t *testing.T) {
		t.Parallel()
		bs := mocks.NewMockBrandService(t)
		bs.EXPECT().IsUserBrandManager(mock.Anything, "u-1", "b-1").Return(true, nil)

		svc := NewAuthzService(bs, nil, nil)
		require.NoError(t, svc.CanViewBrand(ctxWithRole(api.BrandManager), "b-1"))
	})
}

func TestAuthzService_AdminOnlyBrandActions(t *testing.T) {
	t.Parallel()

	actions := []struct {
		name string
		call func(*AuthzService, context.Context) error
	}{
		{"update", func(a *AuthzService, ctx context.Context) error { return a.CanUpdateBrand(ctx, "b-1") }},
		{"delete", func(a *AuthzService, ctx context.Context) error { return a.CanDeleteBrand(ctx, "b-1") }},
		{"assign", func(a *AuthzService, ctx context.Context) error { return a.CanAssignManager(ctx, "b-1") }},
		{"remove", func(a *AuthzService, ctx context.Context) error { return a.CanRemoveManager(ctx, "b-1", "u-2") }},
	}

	for _, tc := range actions {
		t.Run(tc.name+": manager forbidden", func(t *testing.T) {
			t.Parallel()
			svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
			require.ErrorIs(t, tc.call(svc, ctxWithRole(api.BrandManager)), domain.ErrForbidden)
		})
		t.Run(tc.name+": admin allowed", func(t *testing.T) {
			t.Parallel()
			svc := NewAuthzService(mocks.NewMockBrandService(t), nil, nil)
			require.NoError(t, tc.call(svc, ctxWithRole(api.Admin)))
		})
	}
}
