package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

func testBrand() *repository.BrandRow {
	return &repository.BrandRow{ID: "brand-1", Name: "Test Brand"}
}

// expectBrandAudit is a convenience wrapper around expectAudit for BrandService
// tests that use the package-level helper defined in auth_test.go. Callers
// supply the subset of fields they know (Action/EntityType/EntityID are fixed).
func expectBrandAudit(t *testing.T, audit *repomocks.MockAuditRepo, action, entityID, expectedOld, expectedNew string) {
	t.Helper()
	id := entityID
	expectAudit(t, audit, repository.AuditLogRow{
		Action:     action,
		EntityType: AuditEntityTypeBrand,
		EntityID:   &id,
	}, expectedOld, expectedNew)
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

	t.Run("repo error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Create(mock.Anything, "My Brand", (*string)(nil)).
			Return((*repository.BrandRow)(nil), errors.New("db error"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.CreateBrand(context.Background(), "My Brand", nil)
		require.ErrorContains(t, err, "db error")
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
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
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.CreateBrand(context.Background(), "My Brand", nil)
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success trims whitespace and writes audit with full row", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Create(mock.Anything, "Trimmed", (*string)(nil)).
			Return(&repository.BrandRow{
				ID: "b-1", Name: "Trimmed", CreatedAt: created, UpdatedAt: created,
			}, nil)
		expectBrandAudit(t, audit, AuditActionBrandCreate, "b-1", "", `{"name":"Trimmed"}`)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.CreateBrand(context.Background(), "  Trimmed  ", nil)
		require.NoError(t, err)
		require.Equal(t, &domain.Brand{
			ID:        "b-1",
			Name:      "Trimmed",
			LogoURL:   nil,
			CreatedAt: created,
			UpdatedAt: created,
		}, got)
	})
}

func TestBrandService_GetBrand(t *testing.T) {
	t.Parallel()

	t.Run("success maps row to domain", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		logoURL := "https://cdn.example.com/l.png"
		created := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().GetByID(mock.Anything, "b-1").
			Return(&repository.BrandRow{
				ID: "b-1", Name: "Acme", LogoURL: &logoURL,
				CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.GetBrand(context.Background(), "b-1")
		require.NoError(t, err)
		require.Equal(t, &domain.Brand{
			ID:        "b-1",
			Name:      "Acme",
			LogoURL:   &logoURL,
			CreatedAt: created,
			UpdatedAt: created,
		}, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.BrandRow)(nil), errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.GetBrand(context.Background(), "missing")
		require.ErrorIs(t, err, errNotFound)
	})
}

func TestBrandService_ListBrands(t *testing.T) {
	t.Parallel()

	t.Run("all brands when managerID nil", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().List(mock.Anything).
			Return([]*repository.BrandWithManagerCount{
				{ID: "b-1", Name: "Acme", ManagerCount: 2, CreatedAt: created, UpdatedAt: created},
			}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.ListBrands(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, []*domain.BrandListItem{
			{ID: "b-1", Name: "Acme", ManagerCount: 2, CreatedAt: created, UpdatedAt: created},
		}, got)
	})

	t.Run("list error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.ListBrands(context.Background(), nil)
		require.ErrorContains(t, err, "db error")
	})

	t.Run("filter by managerID when set", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListByUser(mock.Anything, "u-1").
			Return([]*repository.BrandWithManagerCount{
				{ID: "b-1", Name: "Acme", ManagerCount: 1, CreatedAt: created, UpdatedAt: created},
			}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		uid := "u-1"
		got, err := svc.ListBrands(context.Background(), &uid)
		require.NoError(t, err)
		require.Equal(t, []*domain.BrandListItem{
			{ID: "b-1", Name: "Acme", ManagerCount: 1, CreatedAt: created, UpdatedAt: created},
		}, got)
	})

	t.Run("listByUser error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListByUser(mock.Anything, "u-1").Return(nil, errors.New("boom"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		uid := "u-1"
		_, err := svc.ListBrands(context.Background(), &uid)
		require.ErrorContains(t, err, "boom")
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

	t.Run("repo error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Update(mock.Anything, "b-1", "New Name", (*string)(nil)).
			Return((*repository.BrandRow)(nil), errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.UpdateBrand(context.Background(), "b-1", "New Name", nil)
		require.ErrorIs(t, err, errNotFound)
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
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
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.UpdateBrand(context.Background(), "b-1", "New Name", nil)
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with new value", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		updated := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Update(mock.Anything, "b-1", "New Name", (*string)(nil)).
			Return(&repository.BrandRow{
				ID: "b-1", Name: "New Name", UpdatedAt: updated,
			}, nil)
		expectBrandAudit(t, audit, AuditActionBrandUpdate, "b-1", "", `{"name":"New Name"}`)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.UpdateBrand(context.Background(), "b-1", "New Name", nil)
		require.NoError(t, err)
		require.Equal(t, &domain.Brand{
			ID:        "b-1",
			Name:      "New Name",
			LogoURL:   nil,
			UpdatedAt: updated,
		}, got)
	})
}

func TestBrandService_DeleteBrand(t *testing.T) {
	t.Parallel()

	t.Run("repo error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Delete(mock.Anything, "b-1").Return(errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		err := svc.DeleteBrand(context.Background(), "b-1")
		require.ErrorIs(t, err, errNotFound)
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Delete(mock.Anything, "b-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		err := svc.DeleteBrand(context.Background(), "b-1")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit without payload", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().Delete(mock.Anything, "b-1").Return(nil)
		expectBrandAudit(t, audit, AuditActionBrandDelete, "b-1", "", "")

		svc := NewBrandService(pool, factory, testBcryptCost)
		require.NoError(t, svc.DeleteBrand(context.Background(), "b-1"))
	})
}

func TestBrandService_ListManagers(t *testing.T) {
	t.Parallel()

	t.Run("success maps rows to domain", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		created1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return([]*repository.BrandManagerRow{
				{UserID: "u-1", Email: "a@example.com", CreatedAt: created1},
				{UserID: "u-2", Email: "b@example.com", CreatedAt: created2},
			}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.ListManagers(context.Background(), "b-1")
		require.NoError(t, err)
		require.Equal(t, []*domain.BrandManager{
			{UserID: "u-1", Email: "a@example.com", AssignedAt: created1},
			{UserID: "u-2", Email: "b@example.com", AssignedAt: created2},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return([]*repository.BrandManagerRow{}, nil)

		svc := NewBrandService(pool, factory, testBcryptCost)
		got, err := svc.ListManagers(context.Background(), "b-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)

		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		brands.EXPECT().ListManagers(mock.Anything, "b-1").
			Return(nil, errors.New("db error"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, err := svc.ListManagers(context.Background(), "b-1")
		require.ErrorContains(t, err, "db error")
	})
}

func TestBrandService_AssignManager(t *testing.T) {
	t.Parallel()

	t.Run("empty email short-circuits", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "")
		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("brand not found aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-missing").
			Return((*repository.BrandRow)(nil), errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-missing", "new@example.com")
		require.ErrorContains(t, err, "get brand")
	})

	t.Run("check user error aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "new@example.com").
			Return(false, errors.New("db error"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "new@example.com")
		require.ErrorContains(t, err, "check user")
	})

	t.Run("get existing user error aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
			Return((*repository.UserRow)(nil), errors.New("db error"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "existing@example.com")
		require.ErrorContains(t, err, "get user")
	})

	t.Run("create user error aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "new@example.com").Return(false, nil)
		users.EXPECT().Create(mock.Anything, "new@example.com", mock.AnythingOfType("string"), string(api.BrandManager)).
			Return((*repository.UserRow)(nil), errors.New("unique violation"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "new@example.com")
		require.ErrorContains(t, err, "create user")
	})

	t.Run("assign manager error aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
			Return(&repository.UserRow{ID: "u-1", Email: "existing@example.com"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-1").
			Return(errors.New("duplicate"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "existing@example.com")
		require.ErrorContains(t, err, "assign manager")
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
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
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
			Return(&repository.UserRow{ID: "u-1", Email: "existing@example.com"}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		_, _, err := svc.AssignManager(context.Background(), "b-1", "existing@example.com")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success existing user without temp password", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		users := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(users)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").Return(true, nil)
		users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
			Return(&repository.UserRow{
				ID: "u-1", Email: "existing@example.com", Role: string(api.BrandManager),
				CreatedAt: created, UpdatedAt: created,
			}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-1").Return(nil)
		expectBrandAudit(t, audit, AuditActionManagerAssign, "b-1", "", `{"email":"existing@example.com"}`)

		svc := NewBrandService(pool, factory, testBcryptCost)
		user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "  Existing@Example.com  ")
		require.NoError(t, err)
		require.Empty(t, tempPass, "existing user should not get temp password")
		require.Equal(t, &domain.User{
			ID:        "u-1",
			Email:     "existing@example.com",
			Role:      api.BrandManager,
			CreatedAt: created,
			UpdatedAt: created,
		}, user)
	})

	t.Run("success new user returns temp password", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		users := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(users)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().GetByID(mock.Anything, "b-1").Return(testBrand(), nil)
		users.EXPECT().ExistsByEmail(mock.Anything, "new@example.com").Return(false, nil)
		users.EXPECT().Create(mock.Anything, "new@example.com", mock.AnythingOfType("string"), string(api.BrandManager)).
			Return(&repository.UserRow{
				ID: "u-new", Email: "new@example.com", Role: string(api.BrandManager),
				CreatedAt: created, UpdatedAt: created,
			}, nil)
		brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-new").Return(nil)
		expectBrandAudit(t, audit, AuditActionManagerAssign, "b-1", "", `{"email":"new@example.com"}`)

		svc := NewBrandService(pool, factory, testBcryptCost)
		user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "new@example.com")
		require.NoError(t, err)
		require.Len(t, tempPass, 12, "temp password should be 12 characters")
		require.Equal(t, &domain.User{
			ID:        "u-new",
			Email:     "new@example.com",
			Role:      api.BrandManager,
			CreatedAt: created,
			UpdatedAt: created,
		}, user)
	})
}

func TestBrandService_RemoveManager(t *testing.T) {
	t.Parallel()

	t.Run("repo error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(errNotFound)

		svc := NewBrandService(pool, factory, testBcryptCost)
		err := svc.RemoveManager(context.Background(), "b-1", "u-2")
		require.ErrorIs(t, err, errNotFound)
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewBrandService(pool, factory, testBcryptCost)
		err := svc.RemoveManager(context.Background(), "b-1", "u-2")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with userId in old_value", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockBrandRepoFactory(t)
		brands := repomocks.NewMockBrandRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewBrandRepo(mock.Anything).Return(brands)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		brands.EXPECT().RemoveManager(mock.Anything, "b-1", "u-2").Return(nil)
		expectBrandAudit(t, audit, AuditActionManagerRemove, "b-1", `{"userId":"u-2"}`, "")

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
