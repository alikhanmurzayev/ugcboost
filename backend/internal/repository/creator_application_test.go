package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestCreatorApplicationRepository_HasActiveByIIN(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT 1 FROM creator_applications WHERE iin = $1 AND status IN ($2,$3,$4,$5) LIMIT 1"

	t.Run("found returns true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
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
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
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
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
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

	const sqlStmt = "INSERT INTO creator_applications (address,birth_date,category_other_text,city_code,first_name,iin,last_name,middle_name,phone,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, status, updated_at"

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		// pgx/stom dereferences *string before binding to the SQL parameter, so
		// WithArgs receives the raw string. AddRow goes through the dbutil
		// scanner which requires the source kind to match the destination kind
		// (*string), so the address/middle/other columns are sourced as pointers.
		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "Авторские ASMR-видео", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "verification").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("Авторские ASMR-видео"), "almaty", created, "Айдана", "app-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", created))

		row := CreatorApplicationRow{
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR-видео"),
			Status:            "verification",
		}
		got, err := repo.Create(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationRow{
			ID:                "app-1",
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR-видео"),
			Status:            "verification",
			CreatedAt:         created,
			UpdatedAt:         created,
		}, got)
	})

	t.Run("translates pgx unique violation on iin index to domain sentinel", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, nil, "almaty", "Айдана", "950515312348", "Муратова", nil, "+77001234567", "verification").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorApplicationsIINActiveIdx})

		_, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:  "Муратова",
			FirstName: "Айдана",
			IIN:       "950515312348",
			BirthDate: birth,
			Phone:     "+77001234567",
			CityCode:  "almaty",
			Address:   pointer.ToString("ул. Абая 1"),
			Status:    "verification",
		})
		require.ErrorIs(t, err, domain.ErrCreatorApplicationDuplicate)
	})

	t.Run("propagates unrelated unique violations as-is", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, nil, "almaty", "Айдана", "950515312348", "Муратова", nil, "+77001234567", "verification").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_idx"})

		_, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:  "Муратова",
			FirstName: "Айдана",
			IIN:       "950515312348",
			BirthDate: birth,
			Phone:     "+77001234567",
			CityCode:  "almaty",
			Address:   pointer.ToString("ул. Абая 1"),
			Status:    "verification",
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorApplicationDuplicate)
	})

	t.Run("address omitted — repo passes nil to insert and reads it back", func(t *testing.T) {
		t.Parallel()
		// Landing form does not collect an address; the row hits the DB with
		// NULL and pgxmock surfaces it as a nil pointer when scanning back.
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(nil, birth, nil, "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "verification").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(nil, birth, nil, "almaty", created, "Айдана", "app-2", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", created))

		got, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: pointer.ToString("Ивановна"),
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			CityCode:   "almaty",
			Status:     "verification",
		})
		require.NoError(t, err)
		require.Nil(t, got.Address)
	})
}

func TestCreatorApplicationRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, status, updated_at FROM creator_applications WHERE id = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("Авторские ASMR"), "almaty", created, "Айдана", "app-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", updated))

		got, err := repo.GetByID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationRow{
			ID:                "app-1",
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR"),
			Status:            "verification",
			CreatedAt:         created,
			UpdatedAt:         updated,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByID(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
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
