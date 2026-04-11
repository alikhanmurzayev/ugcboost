package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

func testBrand() repository.BrandRow {
	return repository.BrandRow{ID: "brand-1", Name: "Test Brand"}
}

// --- CreateBrand ---

func TestCreateBrand_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().Create(mock.Anything, "My Brand", (*string)(nil)).
		Return(repository.BrandRow{ID: "b-1", Name: "My Brand"}, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	b, err := svc.CreateBrand(context.Background(), "My Brand", nil)

	require.NoError(t, err)
	assert.Equal(t, "b-1", b.ID)
	assert.Equal(t, "My Brand", b.Name)
}

func TestCreateBrand_EmptyName(t *testing.T) {
	t.Parallel()
	svc := NewBrandService(nil, nil, testBcryptCost)
	_, err := svc.CreateBrand(context.Background(), "  ", nil)

	var ve *domain.ValidationError
	assert.ErrorAs(t, err, &ve)
}

func TestCreateBrand_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().Create(mock.Anything, "Trimmed", (*string)(nil)).
		Return(repository.BrandRow{ID: "b-1", Name: "Trimmed"}, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	_, err := svc.CreateBrand(context.Background(), "  Trimmed  ", nil)

	require.NoError(t, err)
}

// --- ListBrands ---

func TestListBrands_Admin(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().List(mock.Anything).
		Return([]repository.BrandWithManagerCount{{ID: "b-1"}}, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	list, err := svc.ListBrands(context.Background(), "u-1", "admin")

	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestListBrands_Manager(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().ListByUser(mock.Anything, "u-1").
		Return([]repository.BrandWithManagerCount{{ID: "b-1"}}, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	list, err := svc.ListBrands(context.Background(), "u-1", "brand_manager")

	require.NoError(t, err)
	assert.Len(t, list, 1)
}

// --- UpdateBrand ---

func TestUpdateBrand_Success(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().Update(mock.Anything, "b-1", "New Name", (*string)(nil)).
		Return(repository.BrandRow{ID: "b-1", Name: "New Name"}, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	b, err := svc.UpdateBrand(context.Background(), "b-1", "New Name", nil)

	require.NoError(t, err)
	assert.Equal(t, "New Name", b.Name)
}

func TestUpdateBrand_EmptyName(t *testing.T) {
	t.Parallel()
	svc := NewBrandService(nil, nil, testBcryptCost)
	_, err := svc.UpdateBrand(context.Background(), "b-1", "", nil)

	var ve *domain.ValidationError
	assert.ErrorAs(t, err, &ve)
}

// --- AssignManager ---

func TestAssignManager_NewUser(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	users := mocks.NewMockBrandUserRepo(t)

	brands.EXPECT().GetByID(mock.Anything, "b-1").
		Return(testBrand(), nil)
	users.EXPECT().ExistsByEmail(mock.Anything, "new@example.com").
		Return(false, nil)
	users.EXPECT().Create(mock.Anything, "new@example.com", mock.AnythingOfType("string"), "brand_manager").
		Return(repository.UserRow{ID: "u-new", Email: "new@example.com", Role: "brand_manager"}, nil)
	brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-new").
		Return(nil)

	svc := NewBrandService(brands, users, testBcryptCost)
	user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "new@example.com")

	require.NoError(t, err)
	assert.Equal(t, "u-new", user.ID)
	assert.NotEmpty(t, tempPass, "new user should get a temporary password")
}

func TestAssignManager_ExistingUser(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	users := mocks.NewMockBrandUserRepo(t)

	brands.EXPECT().GetByID(mock.Anything, "b-1").
		Return(testBrand(), nil)
	users.EXPECT().ExistsByEmail(mock.Anything, "existing@example.com").
		Return(true, nil)
	users.EXPECT().GetByEmail(mock.Anything, "existing@example.com").
		Return(repository.UserRow{ID: "u-exist", Email: "existing@example.com", Role: "brand_manager"}, nil)
	brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-exist").
		Return(nil)

	svc := NewBrandService(brands, users, testBcryptCost)
	user, tempPass, err := svc.AssignManager(context.Background(), "b-1", "existing@example.com")

	require.NoError(t, err)
	assert.Equal(t, "u-exist", user.ID)
	assert.Empty(t, tempPass, "existing user should NOT get a temporary password")
}

func TestAssignManager_EmptyEmail(t *testing.T) {
	t.Parallel()
	svc := NewBrandService(nil, nil, testBcryptCost)
	_, _, err := svc.AssignManager(context.Background(), "b-1", "")

	var ve *domain.ValidationError
	assert.ErrorAs(t, err, &ve)
}

func TestAssignManager_NormalizesEmail(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	users := mocks.NewMockBrandUserRepo(t)

	brands.EXPECT().GetByID(mock.Anything, "b-1").
		Return(testBrand(), nil)
	users.EXPECT().ExistsByEmail(mock.Anything, "upper@example.com").
		Return(true, nil)
	users.EXPECT().GetByEmail(mock.Anything, "upper@example.com").
		Return(repository.UserRow{ID: "u-1", Email: "upper@example.com"}, nil)
	brands.EXPECT().AssignManager(mock.Anything, "b-1", "u-1").
		Return(nil)

	svc := NewBrandService(brands, users, testBcryptCost)
	_, _, err := svc.AssignManager(context.Background(), "b-1", "  Upper@Example.com  ")
	require.NoError(t, err)
}

// --- CanViewBrand ---

func TestCanViewBrand_Admin(t *testing.T) {
	t.Parallel()
	svc := NewBrandService(nil, nil, testBcryptCost)
	err := svc.CanViewBrand(context.Background(), "u-1", "admin", "b-1")
	assert.NoError(t, err)
}

func TestCanViewBrand_Manager_IsManager(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().IsManager(mock.Anything, "u-1", "b-1").Return(true, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	err := svc.CanViewBrand(context.Background(), "u-1", "brand_manager", "b-1")
	assert.NoError(t, err)
}

func TestCanViewBrand_Manager_NotManager(t *testing.T) {
	t.Parallel()
	brands := mocks.NewMockBrandRepo(t)
	brands.EXPECT().IsManager(mock.Anything, "u-1", "b-1").Return(false, nil)

	svc := NewBrandService(brands, nil, testBcryptCost)
	err := svc.CanViewBrand(context.Background(), "u-1", "brand_manager", "b-1")
	assert.ErrorIs(t, err, domain.ErrForbidden)
}
