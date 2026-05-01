package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreatorApplicationCategoryRepository_InsertMany(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_categories (application_id,category_code) VALUES ($1,$2),($3,$4)"

	t.Run("empty input short-circuits", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		require.NoError(t, repo.InsertMany(context.Background(), nil))
	})

	t.Run("success inserts rows in order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1", "beauty", "app-1", "fashion").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 2"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryCode: "beauty"},
			{ApplicationID: "app-1", CategoryCode: "fashion"},
		})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1", "beauty", "app-1", "fashion").
			WillReturnError(errors.New("fk violation"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryCode: "beauty"},
			{ApplicationID: "app-1", CategoryCode: "fashion"},
		})
		require.ErrorContains(t, err, "fk violation")
	})
}

func TestCreatorApplicationCategoryRepository_ListByApplicationID(t *testing.T) {
	t.Parallel()

	// category_code lives directly on the link row now, so the read collapses
	// to a single-table SELECT. The deactivation fallback is the handler's
	// job — repo only returns whatever codes were stored at submit time.
	const sqlStmt = "SELECT category_code FROM creator_application_categories WHERE application_id = $1 ORDER BY category_code ASC"

	t.Run("success returns codes in DB order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"category_code"}).
				AddRow("beauty").
				AddRow("fashion"))

		got, err := repo.ListByApplicationID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, []string{"beauty", "fashion"}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-empty").
			WillReturnRows(pgxmock.NewRows([]string{"category_code"}))

		got, err := repo.ListByApplicationID(context.Background(), "app-empty")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByApplicationID(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}
