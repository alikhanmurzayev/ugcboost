package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

func TestBrandRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		logoURL := "https://example.com/logo.png"
		gotSQL, gotArgs := captureQuery(t, db, 2)

		_, _ = repo.Create(context.Background(), "Test Brand", &logoURL)

		require.Equal(t,
			"INSERT INTO brands (logo_url,name) VALUES ($1,$2) RETURNING created_at, id, logo_url, name, updated_at",
			*gotSQL)
		require.Equal(t, []any{logoURL, "Test Brand"}, *gotArgs)
	})
}

func TestBrandRepository_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.GetByID(context.Background(), "brand-1")

		require.Equal(t,
			"SELECT created_at, id, logo_url, name, updated_at FROM brands WHERE id = $1",
			*gotSQL)
		require.Equal(t, []any{"brand-1"}, *gotArgs)
	})
}

func TestBrandRepository_List(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, _ := captureQuery(t, db, 0)

		_, _ = repo.List(context.Background())

		require.Equal(t,
			"SELECT b.id, b.name, b.logo_url, COUNT(bm.id) AS manager_count, b.created_at, b.updated_at FROM brands b LEFT JOIN brand_managers bm ON bm.brand_id = b.id GROUP BY b.id ORDER BY b.created_at DESC",
			*gotSQL)
	})
}

func TestBrandRepository_ListByUser(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.ListByUser(context.Background(), "user-1")

		require.Equal(t,
			"SELECT b.id, b.name, b.logo_url, (SELECT COUNT(*) FROM brand_managers bm2 WHERE bm2.brand_id = b.id) AS manager_count, b.created_at, b.updated_at FROM brands b JOIN brand_managers bm ON bm.brand_id = b.id AND bm.user_id = $1 ORDER BY b.created_at DESC",
			*gotSQL)
		require.Equal(t, []any{"user-1"}, *gotArgs)
	})
}

func TestBrandRepository_Update(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 3)

		_, _ = repo.Update(context.Background(), "brand-1", "New Name", nil)

		require.Equal(t,
			"UPDATE brands SET name = $1, logo_url = $2, updated_at = now() WHERE id = $3 RETURNING created_at, id, logo_url, name, updated_at",
			*gotSQL)
		require.Equal(t, []any{"New Name", (*string)(nil), "brand-1"}, *gotArgs)
	})
}

func TestBrandRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureExec(t, db, 1)

		_ = repo.Delete(context.Background(), "brand-1")

		require.Equal(t,
			"DELETE FROM brands WHERE id = $1",
			*gotSQL)
		require.Equal(t, []any{"brand-1"}, *gotArgs)
	})
}

func TestBrandRepository_AssignManager(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureExec(t, db, 2)

		err := repo.AssignManager(context.Background(), "brand-1", "user-1")
		require.NoError(t, err)

		require.Equal(t,
			"INSERT INTO brand_managers (brand_id,user_id) VALUES ($1,$2)",
			*gotSQL)
		require.Equal(t, []any{"brand-1", "user-1"}, *gotArgs)
	})
}

func TestBrandRepository_RemoveManager(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureExec(t, db, 2)

		_ = repo.RemoveManager(context.Background(), "brand-1", "user-1")

		require.Equal(t,
			"DELETE FROM brand_managers WHERE brand_id = $1 AND user_id = $2",
			*gotSQL)
		require.Equal(t, []any{"brand-1", "user-1"}, *gotArgs)
	})
}

func TestBrandRepository_ListManagers(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.ListManagers(context.Background(), "brand-1")

		require.Equal(t,
			"SELECT bm.user_id, u.email, bm.created_at FROM brand_managers bm JOIN users u ON u.id = bm.user_id WHERE bm.brand_id = $1 ORDER BY bm.created_at ASC",
			*gotSQL)
		require.Equal(t, []any{"brand-1"}, *gotArgs)
	})
}

func TestBrandRepository_GetBrandIDsForUser(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.GetBrandIDsForUser(context.Background(), "user-1")

		require.Equal(t,
			"SELECT brand_id FROM brand_managers WHERE user_id = $1",
			*gotSQL)
		require.Equal(t, []any{"user-1"}, *gotArgs)
	})
}

func TestBrandRepository_IsManager(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := &brandRepository{db: db}
		gotSQL, gotArgs := captureQuery(t, db, 2)

		ok, err := repo.IsManager(context.Background(), "user-1", "brand-1")
		require.Error(t, err) // captureQuery returns non-ErrNoRows error
		require.False(t, ok)

		require.Equal(t,
			"SELECT 1 FROM brand_managers WHERE user_id = $1 AND brand_id = $2 LIMIT 1",
			*gotSQL)
		require.Equal(t, []any{"user-1", "brand-1"}, *gotArgs)
	})
}
