package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
