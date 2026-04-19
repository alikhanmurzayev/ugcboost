package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

func testBrand() *repository.BrandRow {
	return &repository.BrandRow{ID: "brand-1", Name: "Test Brand"}
}

func TestBrandService_CreateBrand(t *testing.T) {
	t.Parallel()

	t.Run("empty name short-circuits", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.CreateBrand(context.Background(), "  ", nil)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("success writes audit in tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().Create(mock.Anything, "My Brand", (*string)(nil)).
			Return(&repository.BrandRow{ID: "b-1", Name: "My Brand"}, nil)

		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionBrandCreate &&
				row.EntityType == AuditEntityTypeBrand &&
				row.EntityID != nil && *row.EntityID == "b-1"
		})).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		b, err := svc.CreateBrand(context.Background(), "My Brand", nil)

		require.NoError(t, err)
		require.Equal(t, "b-1", b.ID)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().Create(mock.Anything, "Trimmed", (*string)(nil)).
			Return(&repository.BrandRow{ID: "b-1", Name: "Trimmed"}, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.CreateBrand(context.Background(), "  Trimmed  ", nil)

		require.NoError(t, err)
	})
}

func TestBrandService_ListBrands(t *testing.T) {
	t.Parallel()

	t.Run("all brands when managerID nil", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().List(mock.Anything).
			Return([]*repository.BrandWithManagerCount{{ID: "b-1"}}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		list, err := svc.ListBrands(context.Background(), nil)

		require.NoError(t, err)
		require.Len(t, list, 1)
	})

	t.Run("filters by manager when managerID set", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListByUser(mock.Anything, "u-1").
			Return([]*repository.BrandWithManagerCount{{ID: "b-1"}}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		uid := "u-1"
		list, err := svc.ListBrands(context.Background(), &uid)

		require.NoError(t, err)
		require.Len(t, list, 1)
	})
}

func TestBrandService_UpdateBrand(t *testing.T) {
	t.Parallel()

	t.Run("empty name short-circuits", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.UpdateBrand(context.Background(), "b-1", "", nil)

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("success writes audit in tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().Update(mock.Anything, "b-1", "New Name", (*string)(nil)).
			Return(&repository.BrandRow{ID: "b-1", Name: "New Name"}, nil)
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionBrandUpdate && row.EntityType == AuditEntityTypeBrand
		})).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		b, err := svc.UpdateBrand(context.Background(), "b-1", "New Name", nil)

		require.NoError(t, err)
		require.Equal(t, "New Name", b.Name)
	})
}

func TestBrandService_DeleteBrand(t *testing.T) {
	t.Parallel()

	t.Run("success writes audit in tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().Delete(mock.Anything, "b-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionBrandDelete && row.EntityID != nil && *row.EntityID == "b-1"
		})).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		require.NoError(t, svc.DeleteBrand(context.Background(), "b-1"))
	})
}

func TestBrandService_AssignManager(t *testing.T) {
	t.Parallel()

	t.Run("new user", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		users := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(users)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().GetByID(mock.Anything, "b-1").
			Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "new@example.com").
			Return(false, nil)
		users.EXPECT().Create(mock.Anything, "new@example.com", mock.AnythingOfType("string"), "brand_manager").
			Return(&repository.UserRow{ID: "u-new", Email: "new@example.com", Role: "brand_manager"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-new").
			Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionManagerAssign
		})).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "new@example.com")

		require.NoError(t, err)
		require.Equal(t, "u-new", user.ID)
		require.NotEmpty(t, tempPass, "new user should get a temporary password")
	})

	t.Run("existing user", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		users := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(users)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().GetByID(mock.Anything, "b-1").
			Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").
			Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
			Return(&repository.UserRow{ID: "u-exist", Email: "existing@example.com", Role: "brand_manager"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-exist").
			Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "existing@example.com")

		require.NoError(t, err)
		require.Equal(t, "u-exist", user.ID)
		require.Empty(t, tempPass, "existing user should NOT get a temporary password")
	})

	t.Run("empty email", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "")

		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("normalizes email", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		users := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(users)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().GetByID(mock.Anything, "b-1").
			Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "upper@example.com").
			Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "upper@example.com").
			Return(&repository.UserRow{ID: "u-1", Email: "upper@example.com"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-1").
			Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "  Upper@Example.com  ")
		require.NoError(t, err)
	})
}

func TestBrandService_RemoveManager(t *testing.T) {
	t.Parallel()

	t.Run("success writes audit in tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionManagerRemove
		})).Return(nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		require.NoError(t, svc.RemoveManager(context.Background(), "b-1", "u-2"))
	})
}

func TestBrandService_IsUserBrandManager(t *testing.T) {
	t.Parallel()

	t.Run("repo error wraps", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().IsManager(mock.Anything, "u-1", "b-1").Return(false, errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		ok, err := svc.IsUserBrandManager(context.Background(), "u-1", "b-1")
		require.ErrorContains(t, err, "check manager")
		require.False(t, ok)
	})

	t.Run("false when not a manager", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().IsManager(mock.Anything, "u-1", "b-1").Return(false, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		ok, err := svc.IsUserBrandManager(context.Background(), "u-1", "b-1")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("true when manager", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().IsManager(mock.Anything, "u-1", "b-1").Return(true, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		ok, err := svc.IsUserBrandManager(context.Background(), "u-1", "b-1")
		require.NoError(t, err)
		require.True(t, ok)
	})
}
