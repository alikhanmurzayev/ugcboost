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

func TestCreatorApplicationRepository_HasActiveByIIN(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT 1 FROM creator_applications WHERE iin = $1 AND status IN ($2,$3,$4) LIMIT 1"

	t.Run("found returns true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "pending", "approved", "blocked").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("not found returns false without error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "pending", "approved", "blocked").
			WillReturnError(pgx.ErrNoRows)

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "pending", "approved", "blocked").
			WillReturnError(errors.New("db exploded"))

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.ErrorContains(t, err, "db exploded")
		require.False(t, ok)
		// sql.ErrNoRows should not be surfaced for this case.
		require.NotErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestCreatorApplicationRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_applications (address,birth_date,city,first_name,iin,last_name,middle_name,phone) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING address, birth_date, city, created_at, first_name, id, iin, last_name, middle_name, phone, status, updated_at"

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		middle := "Ивановна"
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "Алматы", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "city", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow("ул. Абая 1", birth, "Алматы", created, "Айдана", "app-1", "950515312348", "Муратова", &middle, "+77001234567", "pending", created))

		row := CreatorApplicationRow{
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: &middle,
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Address:    "ул. Абая 1",
		}
		got, err := repo.Create(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationRow{
			ID:         "app-1",
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: &middle,
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			City:       "Алматы",
			Address:    "ул. Абая 1",
			Status:     "pending",
			CreatedAt:  created,
			UpdatedAt:  created,
		}, got)
	})

	t.Run("propagates unique violation style error for duplicate iin", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "Алматы", "Айдана", "950515312348", "Муратова", nil, "+77001234567").
			WillReturnError(errors.New("duplicate key value violates unique constraint creator_applications_iin_active_idx"))

		_, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:  "Муратова",
			FirstName: "Айдана",
			IIN:       "950515312348",
			BirthDate: birth,
			Phone:     "+77001234567",
			City:      "Алматы",
			Address:   "ул. Абая 1",
		})
		require.ErrorContains(t, err, "creator_applications_iin_active_idx")
	})
}

func TestCreatorApplicationRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM creator_applications WHERE id = $1"

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		require.NoError(t, repo.DeleteForTests(context.Background(), "app-1"))
	})

	t.Run("missing returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		err := repo.DeleteForTests(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}
