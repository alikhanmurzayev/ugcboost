package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestDictionaryRepository_ListActive(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT active, code, created_at, id, name, sort_order FROM categories WHERE active = $1 ORDER BY sort_order, code"

	t.Run("success returns rows ordered by sort_order then code", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &dictionaryRepository{db: mock}
		created := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(true).
			WillReturnRows(pgxmock.NewRows([]string{"active", "code", "created_at", "id", "name", "sort_order"}).
				AddRow(true, "fashion", created, "c-1", "Мода / Стиль", 10).
				AddRow(true, "beauty", created, "c-2", "Бьюти (макияж, уход)", 20))

		got, err := repo.ListActive(context.Background(), TableCategories)
		require.NoError(t, err)
		require.Equal(t, []*DictionaryEntryRow{
			{ID: "c-1", Code: "fashion", Name: "Мода / Стиль", Active: true, SortOrder: 10, CreatedAt: created},
			{ID: "c-2", Code: "beauty", Name: "Бьюти (макияж, уход)", Active: true, SortOrder: 20, CreatedAt: created},
		}, got)
	})

	t.Run("propagates query error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &dictionaryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(true).
			WillReturnError(errors.New("db down"))

		_, err := repo.ListActive(context.Background(), TableCategories)
		require.ErrorContains(t, err, "db down")
	})
}

func TestDictionaryRepository_GetActiveByCodes(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT active, code, created_at, id, name, sort_order FROM cities WHERE code IN ($1,$2) AND active = $3"

	t.Run("empty codes short-circuits without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &dictionaryRepository{db: mock}

		rows, err := repo.GetActiveByCodes(context.Background(), TableCities, nil)
		require.NoError(t, err)
		require.Nil(t, rows)
	})

	t.Run("success maps rows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &dictionaryRepository{db: mock}
		created := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("almaty", "astana", true).
			WillReturnRows(pgxmock.NewRows([]string{"active", "code", "created_at", "id", "name", "sort_order"}).
				AddRow(true, "almaty", created, "c-1", "Алматы", 10).
				AddRow(true, "astana", created, "c-2", "Астана", 20))

		got, err := repo.GetActiveByCodes(context.Background(), TableCities, []string{"almaty", "astana"})
		require.NoError(t, err)
		require.Equal(t, []*DictionaryEntryRow{
			{ID: "c-1", Code: "almaty", Name: "Алматы", Active: true, SortOrder: 10, CreatedAt: created},
			{ID: "c-2", Code: "astana", Name: "Астана", Active: true, SortOrder: 20, CreatedAt: created},
		}, got)
	})

	t.Run("propagates query error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &dictionaryRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("almaty", "astana", true).
			WillReturnError(errors.New("db down"))

		_, err := repo.GetActiveByCodes(context.Background(), TableCities, []string{"almaty", "astana"})
		require.ErrorContains(t, err, "db down")
	})
}
