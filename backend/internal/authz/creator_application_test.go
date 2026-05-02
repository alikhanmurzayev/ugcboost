package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestAuthzService_CanViewCreatorApplication(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanViewCreatorApplication(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanViewCreatorApplication(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanViewCreatorApplication(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanListCreatorApplications(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanListCreatorApplications(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanListCreatorApplications(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanListCreatorApplications(ctxWithRole(api.Admin)))
	})
}

func TestAuthzService_CanGetCreatorApplicationsCounts(t *testing.T) {
	t.Parallel()

	t.Run("manager forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanGetCreatorApplicationsCounts(ctxWithRole(api.BrandManager)), domain.ErrForbidden)
	})

	t.Run("missing role forbidden", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.ErrorIs(t, svc.CanGetCreatorApplicationsCounts(context.Background()), domain.ErrForbidden)
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		svc := NewAuthzService(mocks.NewMockBrandService(t))
		require.NoError(t, svc.CanGetCreatorApplicationsCounts(ctxWithRole(api.Admin)))
	})
}
