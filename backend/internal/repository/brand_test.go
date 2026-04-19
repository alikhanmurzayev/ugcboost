package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestBrandRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO brands (logo_url,name) VALUES ($1,$2) RETURNING created_at, id, logo_url, name, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logoURL := "https://example.com/logo.png"
		createdAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(logoURL, "Acme").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "logo_url", "name", "updated_at"}).
				AddRow(createdAt, "b-1", &logoURL, "Acme", createdAt))

		got, err := repo.Create(context.Background(), "Acme", &logoURL)
		require.NoError(t, err)
		require.Equal(t, &BrandRow{
			ID:        "b-1",
			Name:      "Acme",
			LogoURL:   &logoURL,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logoURL := "https://example.com/logo.png"

		mock.ExpectQuery(sqlStmt).
			WithArgs(logoURL, "Acme").
			WillReturnError(errors.New("unique violation"))

		_, err := repo.Create(context.Background(), "Acme", &logoURL)
		require.ErrorContains(t, err, "unique violation")
	})
}

func TestBrandRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT created_at, id, logo_url, name, updated_at FROM brands WHERE id = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logoURL := "https://cdn.example.com/l.png"
		createdAt := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("b-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "logo_url", "name", "updated_at"}).
				AddRow(createdAt, "b-1", &logoURL, "Acme", updatedAt))

		got, err := repo.GetByID(context.Background(), "b-1")
		require.NoError(t, err)
		require.Equal(t, &BrandRow{
			ID:        "b-1",
			Name:      "Acme",
			LogoURL:   &logoURL,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("b-1").
			WillReturnError(errors.New("timeout"))

		_, err := repo.GetByID(context.Background(), "b-1")
		require.ErrorContains(t, err, "timeout")
	})
}

func TestBrandRepository_List(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT b.id, b.name, b.logo_url, COUNT(bm.id) AS manager_count, b.created_at, b.updated_at FROM brands b LEFT JOIN brand_managers bm ON bm.brand_id = b.id GROUP BY b.id ORDER BY b.created_at DESC"

	t.Run("success maps rows to structs", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logoA := "https://cdn.example.com/a.png"
		created1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "logo_url", "manager_count", "created_at", "updated_at"}).
				AddRow("b-1", "Acme", &logoA, 2, created1, created1).
				AddRow("b-2", "Beta", (*string)(nil), 0, created2, created2))

		got, err := repo.List(context.Background())
		require.NoError(t, err)
		require.Equal(t, []*BrandWithManagerCount{
			{ID: "b-1", Name: "Acme", LogoURL: &logoA, ManagerCount: 2, CreatedAt: created1, UpdatedAt: created1},
			{ID: "b-2", Name: "Beta", LogoURL: nil, ManagerCount: 0, CreatedAt: created2, UpdatedAt: created2},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "logo_url", "manager_count", "created_at", "updated_at"}))

		got, err := repo.List(context.Background())
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WillReturnError(errors.New("db error"))

		_, err := repo.List(context.Background())
		require.ErrorContains(t, err, "db error")
	})
}

func TestBrandRepository_ListByUser(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT b.id, b.name, b.logo_url, (SELECT COUNT(*) FROM brand_managers bm2 WHERE bm2.brand_id = b.id) AS manager_count, b.created_at, b.updated_at FROM brands b JOIN brand_managers bm ON bm.brand_id = b.id AND bm.user_id = $1 ORDER BY b.created_at DESC"

	t.Run("success maps rows to structs", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logo := "https://cdn.example.com/a.png"
		created1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1").
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "logo_url", "manager_count", "created_at", "updated_at"}).
				AddRow("b-1", "Acme", &logo, 3, created1, created1).
				AddRow("b-2", "Beta", (*string)(nil), 1, created2, created2))

		got, err := repo.ListByUser(context.Background(), "u-1")
		require.NoError(t, err)
		require.Equal(t, []*BrandWithManagerCount{
			{ID: "b-1", Name: "Acme", LogoURL: &logo, ManagerCount: 3, CreatedAt: created1, UpdatedAt: created1},
			{ID: "b-2", Name: "Beta", LogoURL: nil, ManagerCount: 1, CreatedAt: created2, UpdatedAt: created2},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-empty").
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "logo_url", "manager_count", "created_at", "updated_at"}))

		got, err := repo.ListByUser(context.Background(), "u-empty")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1").
			WillReturnError(errors.New("boom"))

		_, err := repo.ListByUser(context.Background(), "u-1")
		require.ErrorContains(t, err, "boom")
	})
}

func TestBrandRepository_Update(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE brands SET name = $1, logo_url = $2, updated_at = now() WHERE id = $3 RETURNING created_at, id, logo_url, name, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		logoURL := "https://cdn.example.com/new.png"
		createdAt := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("New Name", &logoURL, "b-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "logo_url", "name", "updated_at"}).
				AddRow(createdAt, "b-1", &logoURL, "New Name", updatedAt))

		got, err := repo.Update(context.Background(), "b-1", "New Name", &logoURL)
		require.NoError(t, err)
		require.Equal(t, &BrandRow{
			ID:        "b-1",
			Name:      "New Name",
			LogoURL:   &logoURL,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("New Name", (*string)(nil), "missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.Update(context.Background(), "missing", "New Name", nil)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("New Name", (*string)(nil), "b-1").
			WillReturnError(errors.New("constraint error"))

		_, err := repo.Update(context.Background(), "b-1", "New Name", nil)
		require.ErrorContains(t, err, "constraint error")
	})
}

func TestBrandRepository_Delete(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM brands WHERE id = $1"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.Delete(context.Background(), "b-1")
		require.NoError(t, err)
	})

	t.Run("no rows affected wraps sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.Delete(context.Background(), "missing")
		require.ErrorContains(t, err, "brand not found")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1").
			WillReturnError(errors.New("fk violation"))

		err := repo.Delete(context.Background(), "b-1")
		require.ErrorContains(t, err, "fk violation")
	})
}

func TestBrandRepository_AssignManager(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO brand_managers (brand_id,user_id) VALUES ($1,$2)"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1", "u-1").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

		err := repo.AssignManager(context.Background(), "b-1", "u-1")
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1", "u-1").
			WillReturnError(errors.New("duplicate assignment"))

		err := repo.AssignManager(context.Background(), "b-1", "u-1")
		require.ErrorContains(t, err, "duplicate assignment")
	})
}

func TestBrandRepository_RemoveManager(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM brand_managers WHERE brand_id = $1 AND user_id = $2"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1", "u-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.RemoveManager(context.Background(), "b-1", "u-1")
		require.NoError(t, err)
	})

	t.Run("no rows affected wraps sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1", "u-missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.RemoveManager(context.Background(), "b-1", "u-missing")
		require.ErrorContains(t, err, "manager assignment not found")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("b-1", "u-1").
			WillReturnError(errors.New("db error"))

		err := repo.RemoveManager(context.Background(), "b-1", "u-1")
		require.ErrorContains(t, err, "db error")
	})
}

func TestBrandRepository_ListManagers(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT bm.user_id, u.email, bm.created_at FROM brand_managers bm JOIN users u ON u.id = bm.user_id WHERE bm.brand_id = $1 ORDER BY bm.created_at ASC"

	t.Run("success maps rows to structs", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}
		created1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		created2 := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("b-1").
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "email", "created_at"}).
				AddRow("u-1", "alice@example.com", created1).
				AddRow("u-2", "bob@example.com", created2))

		got, err := repo.ListManagers(context.Background(), "b-1")
		require.NoError(t, err)
		require.Equal(t, []*BrandManagerRow{
			{UserID: "u-1", Email: "alice@example.com", CreatedAt: created1},
			{UserID: "u-2", Email: "bob@example.com", CreatedAt: created2},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("b-1").
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "email", "created_at"}))

		got, err := repo.ListManagers(context.Background(), "b-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("b-1").
			WillReturnError(errors.New("db error"))

		_, err := repo.ListManagers(context.Background(), "b-1")
		require.ErrorContains(t, err, "db error")
	})
}

func TestBrandRepository_GetBrandIDsForUser(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT brand_id FROM brand_managers WHERE user_id = $1"

	t.Run("success maps rows to slice", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1").
			WillReturnRows(pgxmock.NewRows([]string{"brand_id"}).
				AddRow("b-1").
				AddRow("b-2"))

		got, err := repo.GetBrandIDsForUser(context.Background(), "u-1")
		require.NoError(t, err)
		require.Equal(t, []string{"b-1", "b-2"}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-empty").
			WillReturnRows(pgxmock.NewRows([]string{"brand_id"}))

		got, err := repo.GetBrandIDsForUser(context.Background(), "u-empty")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1").
			WillReturnError(errors.New("db error"))

		_, err := repo.GetBrandIDsForUser(context.Background(), "u-1")
		require.ErrorContains(t, err, "db error")
	})
}

func TestBrandRepository_IsManager(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT 1 FROM brand_managers WHERE user_id = $1 AND brand_id = $2 LIMIT 1"

	t.Run("manager returns true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1", "b-1").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))

		ok, err := repo.IsManager(context.Background(), "u-1", "b-1")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("no row returns false without error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1", "b-missing").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}))

		ok, err := repo.IsManager(context.Background(), "u-1", "b-missing")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &brandRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-1", "b-1").
			WillReturnError(errors.New("db unavailable"))

		ok, err := repo.IsManager(context.Background(), "u-1", "b-1")
		require.ErrorContains(t, err, "db unavailable")
		require.False(t, ok)
	})
}

