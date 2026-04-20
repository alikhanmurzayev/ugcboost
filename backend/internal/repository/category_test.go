package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCategoryRepository_GetActiveByCodes(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT active, code, created_at, id, name FROM categories WHERE code IN ($1,$2) AND active = $3"

	t.Run("empty codes short-circuits without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &categoryRepository{db: mock}

		rows, err := repo.GetActiveByCodes(context.Background(), nil)
		require.NoError(t, err)
		require.Nil(t, rows)
	})

	t.Run("success maps rows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &categoryRepository{db: mock}
		created := time.Date(2026, 4, 20, 17, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("beauty", "fashion", true).
			WillReturnRows(pgxmock.NewRows([]string{"active", "code", "created_at", "id", "name"}).
				AddRow(true, "beauty", created, "c-1", "Бьюти").
				AddRow(true, "fashion", created, "c-2", "Мода"))

		got, err := repo.GetActiveByCodes(context.Background(), []string{"beauty", "fashion"})
		require.NoError(t, err)
		require.Equal(t, []*CategoryRow{
			{ID: "c-1", Code: "beauty", Name: "Бьюти", Active: true, CreatedAt: created},
			{ID: "c-2", Code: "fashion", Name: "Мода", Active: true, CreatedAt: created},
		}, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &categoryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("beauty", "fashion", true).
			WillReturnError(errors.New("db down"))

		_, err := repo.GetActiveByCodes(context.Background(), []string{"beauty", "fashion"})
		require.ErrorContains(t, err, "db down")
	})
}
