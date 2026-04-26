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

	const sqlStmt = "INSERT INTO creator_application_categories (application_id,category_id) VALUES ($1,$2),($3,$4)"

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
			WithArgs("app-1", "cat-1", "app-1", "cat-2").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 2"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryID: "cat-1"},
			{ApplicationID: "app-1", CategoryID: "cat-2"},
		})
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1", "cat-1", "app-1", "cat-2").
			WillReturnError(errors.New("fk violation"))

		err := repo.InsertMany(context.Background(), []CreatorApplicationCategoryRow{
			{ApplicationID: "app-1", CategoryID: "cat-1"},
			{ApplicationID: "app-1", CategoryID: "cat-2"},
		})
		require.ErrorContains(t, err, "fk violation")
	})
}

func TestCreatorApplicationCategoryRepository_ListByApplicationID(t *testing.T) {
	t.Parallel()

	// Strict literal — ensures the JOIN against categories and the
	// (sort_order, code) ordering stay glued to the read aggregate's contract.
	// squirrel renders Join() as plain "JOIN" (not "INNER JOIN"), which is
	// equivalent in Postgres.
	const sqlStmt = "SELECT c.code, c.name, c.sort_order FROM creator_application_categories cac JOIN categories c ON c.id = cac.category_id WHERE cac.application_id = $1 ORDER BY c.sort_order ASC, c.code ASC"

	t.Run("success maps rows in DB order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"code", "name", "sort_order"}).
				AddRow("beauty", "Красота", 10).
				AddRow("fashion", "Мода", 20))

		got, err := repo.ListByApplicationID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, []*CreatorApplicationCategoryDetailRow{
			{Code: "beauty", Name: "Красота", SortOrder: 10},
			{Code: "fashion", Name: "Мода", SortOrder: 20},
		}, got)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-empty").
			WillReturnRows(pgxmock.NewRows([]string{"code", "name", "sort_order"}))

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
