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

func TestCreatorCategoryRepository_ListByCreatorID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT category_code FROM creator_categories WHERE creator_id = $1 ORDER BY category_code ASC"

	t.Run("happy: returns codes in ascending order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"category_code"}).
				AddRow("fashion").
				AddRow("food").
				AddRow("lifestyle"))

		got, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Equal(t, []string{"fashion", "food", "lifestyle"}, got)
	})

	t.Run("empty result returns nil slice", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"category_code"}))

		got, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorCategoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByCreatorID(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}
