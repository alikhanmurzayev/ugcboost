package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

// --- Brand Create ---

func TestBrandCreate_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	logoURL := "https://example.com/logo.png"
	gotSQL, gotArgs := captureQuery(t, db, 2)

	_, _ = repo.Create(context.Background(), "Test Brand", &logoURL)

	assert.Equal(t,
		"INSERT INTO brands (logo_url,name) VALUES ($1,$2) RETURNING created_at, id, logo_url, name, updated_at",
		*gotSQL)
	assert.Equal(t, []any{logoURL, "Test Brand"}, *gotArgs)
}

// --- Brand GetByID ---

func TestBrandGetByID_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.GetByID(context.Background(), "brand-1")

	assert.Equal(t,
		"SELECT created_at, id, logo_url, name, updated_at FROM brands WHERE id = $1",
		*gotSQL)
	assert.Equal(t, []any{"brand-1"}, *gotArgs)
}

// --- Brand List ---

func TestBrandList_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, _ := captureQuery(t, db, 0)

	_, _ = repo.List(context.Background())

	assert.Equal(t,
		"SELECT b.id, b.name, b.logo_url, COUNT(bm.id) AS manager_count, b.created_at, b.updated_at FROM brands b LEFT JOIN brand_managers bm ON bm.brand_id = b.id GROUP BY b.id ORDER BY b.created_at DESC",
		*gotSQL)
}

// --- Brand ListByUser ---

func TestBrandListByUser_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.ListByUser(context.Background(), "user-1")

	assert.Equal(t,
		"SELECT b.id, b.name, b.logo_url, (SELECT COUNT(*) FROM brand_managers bm2 WHERE bm2.brand_id = b.id) AS manager_count, b.created_at, b.updated_at FROM brands b JOIN brand_managers bm ON bm.brand_id = b.id AND bm.user_id = $1 ORDER BY b.created_at DESC",
		*gotSQL)
	assert.Equal(t, []any{"user-1"}, *gotArgs)
}

// --- Brand Update ---

func TestBrandUpdate_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 3)

	_, _ = repo.Update(context.Background(), "brand-1", "New Name", nil)

	assert.Equal(t,
		"UPDATE brands SET name = $1, logo_url = $2, updated_at = now() WHERE id = $3 RETURNING created_at, id, logo_url, name, updated_at",
		*gotSQL)
	assert.Equal(t, []any{"New Name", (*string)(nil), "brand-1"}, *gotArgs)
}

// --- Brand Delete ---

func TestBrandDelete_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureExec(t, db, 1)

	err := repo.Delete(context.Background(), "brand-1")
	_ = err

	assert.Equal(t,
		"DELETE FROM brands WHERE id = $1",
		*gotSQL)
	assert.Equal(t, []any{"brand-1"}, *gotArgs)
}

// --- AssignManager ---

func TestAssignManager_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureExec(t, db, 2)

	err := repo.AssignManager(context.Background(), "brand-1", "user-1")
	assert.NoError(t, err)

	assert.Equal(t,
		"INSERT INTO brand_managers (brand_id,user_id) VALUES ($1,$2)",
		*gotSQL)
	assert.Equal(t, []any{"brand-1", "user-1"}, *gotArgs)
}

// --- RemoveManager ---

func TestRemoveManager_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureExec(t, db, 2)

	_ = repo.RemoveManager(context.Background(), "brand-1", "user-1")

	assert.Equal(t,
		"DELETE FROM brand_managers WHERE brand_id = $1 AND user_id = $2",
		*gotSQL)
	assert.Equal(t, []any{"brand-1", "user-1"}, *gotArgs)
}

// --- ListManagers ---

func TestListManagers_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.ListManagers(context.Background(), "brand-1")

	assert.Equal(t,
		"SELECT bm.user_id, u.email, bm.created_at FROM brand_managers bm JOIN users u ON u.id = bm.user_id WHERE bm.brand_id = $1 ORDER BY bm.created_at ASC",
		*gotSQL)
	assert.Equal(t, []any{"brand-1"}, *gotArgs)
}

// --- IsManager ---

func TestIsManager_SQL(t *testing.T) {
	t.Parallel()
	db := mocks.NewMockDB(t)
	repo := NewBrandRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 2)

	ok, err := repo.IsManager(context.Background(), "user-1", "brand-1")
	assert.Error(t, err) // captureQuery returns non-ErrNoRows error
	assert.False(t, ok)

	assert.Equal(t,
		"SELECT 1 FROM brand_managers WHERE user_id = $1 AND brand_id = $2 LIMIT 1",
		*gotSQL)
	assert.Equal(t, []any{"user-1", "brand-1"}, *gotArgs)
}
