package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestCampaignRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO campaigns (name,tma_url) VALUES ($1,$2) RETURNING created_at, id, is_deleted, name, tma_url, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo X", "https://tma.ugcboost.kz/tz/abc", createdAt))

		got, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.NoError(t, err)
		require.Equal(t, &CampaignRow{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: false,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, got)
	})

	t.Run("name taken returns ErrCampaignNameTaken", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CampaignsNameActiveUnique})

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorIs(t, err, domain.ErrCampaignNameTaken)
	})

	t.Run("unrelated 23505 propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "campaigns_other_unique"}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(pgErr)

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.NotErrorIs(t, err, domain.ErrCampaignNameTaken)
		require.ErrorIs(t, err, pgErr)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM campaigns WHERE id = $1"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("c-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.DeleteForTests(context.Background(), "c-1")
		require.NoError(t, err)
	})

	t.Run("no rows affected returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("c-1").
			WillReturnError(errors.New("db error"))

		err := repo.DeleteForTests(context.Background(), "c-1")
		require.ErrorContains(t, err, "db error")
	})
}
