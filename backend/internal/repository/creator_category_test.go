package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreatorCategoryRepository_InsertMany(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_categories (category_code,creator_id) VALUES ($1,$2),($3,$4),($5,$6)"

	t.Run("empty input is a no-op", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}
		require.NoError(t, repo.InsertMany(context.Background(), nil))
		require.NoError(t, repo.InsertMany(context.Background(), []CreatorCategoryRow{}))
	})

	t.Run("happy: writes rows in a single INSERT", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("fashion", "creator-1", "lifestyle", "creator-1", "food", "creator-1").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 3"))

		require.NoError(t, repo.InsertMany(context.Background(), []CreatorCategoryRow{
			{CreatorID: "creator-1", CategoryCode: "fashion"},
			{CreatorID: "creator-1", CategoryCode: "lifestyle"},
			{CreatorID: "creator-1", CategoryCode: "food"},
		}))
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("fashion", "creator-1", "lifestyle", "creator-1", "food", "creator-1").
			WillReturnError(errors.New("db down"))

		err := repo.InsertMany(context.Background(), []CreatorCategoryRow{
			{CreatorID: "creator-1", CategoryCode: "fashion"},
			{CreatorID: "creator-1", CategoryCode: "lifestyle"},
			{CreatorID: "creator-1", CategoryCode: "food"},
		})
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorCategoryRepository_ListByCreatorIDs(t *testing.T) {
	t.Parallel()

	const sqlStmtSingle = "SELECT creator_id, category_code FROM creator_categories WHERE creator_id IN ($1) ORDER BY creator_id ASC, category_code ASC"
	const sqlStmtPair = "SELECT creator_id, category_code FROM creator_categories WHERE creator_id IN ($1,$2) ORDER BY creator_id ASC, category_code ASC"

	t.Run("empty input short-circuits without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		got, err := repo.ListByCreatorIDs(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("happy single creator: returns codes in ascending order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmtSingle).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"creator_id", "category_code"}).
				AddRow("creator-1", "fashion").
				AddRow("creator-1", "food").
				AddRow("creator-1", "lifestyle"))

		got, err := repo.ListByCreatorIDs(context.Background(), []string{"creator-1"})
		require.NoError(t, err)
		require.Equal(t, map[string][]string{
			"creator-1": {"fashion", "food", "lifestyle"},
		}, got)
	})

	t.Run("happy pair: groups codes by creator id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmtPair).
			WithArgs("creator-1", "creator-2").
			WillReturnRows(pgxmock.NewRows([]string{"creator_id", "category_code"}).
				AddRow("creator-1", "beauty").
				AddRow("creator-1", "fashion").
				AddRow("creator-2", "food"))

		got, err := repo.ListByCreatorIDs(context.Background(), []string{"creator-1", "creator-2"})
		require.NoError(t, err)
		require.Equal(t, map[string][]string{
			"creator-1": {"beauty", "fashion"},
			"creator-2": {"food"},
		}, got)
	})

	t.Run("missing creator id surfaces as no-key in map", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmtPair).
			WithArgs("creator-1", "creator-empty").
			WillReturnRows(pgxmock.NewRows([]string{"creator_id", "category_code"}).
				AddRow("creator-1", "beauty"))

		got, err := repo.ListByCreatorIDs(context.Background(), []string{"creator-1", "creator-empty"})
		require.NoError(t, err)
		require.Equal(t, map[string][]string{"creator-1": {"beauty"}}, got)
		require.NotContains(t, got, "creator-empty")
	})

	t.Run("propagates query error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmtPair).
			WithArgs("creator-1", "creator-2").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByCreatorIDs(context.Background(), []string{"creator-1", "creator-2"})
		require.ErrorContains(t, err, "db down")
	})
}
