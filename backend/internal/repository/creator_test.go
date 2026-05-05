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

func TestCreatorRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creators (address,birth_date,category_other_text,city_code,first_name,iin,last_name,middle_name,phone,source_application_id,telegram_first_name,telegram_last_name,telegram_user_id,telegram_username) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, source_application_id, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username, updated_at"

	birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	rowFull := CreatorRow{
		IIN:                 "950515312348",
		LastName:            "Муратова",
		FirstName:           "Айдана",
		MiddleName:          pointer.ToString("Ивановна"),
		BirthDate:           birth,
		Phone:               "+77001234567",
		CityCode:            "almaty",
		Address:             pointer.ToString("ул. Абая 1"),
		CategoryOtherText:   pointer.ToString("ASMR"),
		TelegramUserID:      9000000001,
		TelegramUsername:    pointer.ToString("aidana"),
		TelegramFirstName:   pointer.ToString("Aidana"),
		TelegramLastName:    pointer.ToString("M."),
		SourceApplicationID: "app-1",
	}

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("ASMR"), "almaty", created, "Айдана", "creator-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "app-1", pointer.ToString("Aidana"), pointer.ToString("M."), int64(9000000001), pointer.ToString("aidana"), created))

		got, err := repo.Create(context.Background(), rowFull)
		require.NoError(t, err)
		require.Equal(t, &CreatorRow{
			ID:                  "creator-1",
			IIN:                 "950515312348",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			MiddleName:          pointer.ToString("Ивановна"),
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			Address:             pointer.ToString("ул. Абая 1"),
			CategoryOtherText:   pointer.ToString("ASMR"),
			TelegramUserID:      9000000001,
			TelegramUsername:    pointer.ToString("aidana"),
			TelegramFirstName:   pointer.ToString("Aidana"),
			TelegramLastName:    pointer.ToString("M."),
			SourceApplicationID: "app-1",
			CreatedAt:           created,
			UpdatedAt:           created,
		}, got)
	})

	t.Run("nullable columns are passed as nil and read back as nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		row := CreatorRow{
			IIN:                 "950515312349",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			TelegramUserID:      9000000002,
			SourceApplicationID: "app-2",
		}

		mock.ExpectQuery(sqlStmt).
			WithArgs(nil, birth, nil, "almaty", "Айдана", "950515312349", "Муратова", nil, "+77001234567", "app-2", nil, nil, int64(9000000002), nil).
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(nil, birth, nil, "almaty", created, "Айдана", "creator-2", "950515312349", "Муратова", nil, "+77001234567", "app-2", nil, nil, int64(9000000002), nil, created))

		got, err := repo.Create(context.Background(), row)
		require.NoError(t, err)
		require.Nil(t, got.MiddleName)
		require.Nil(t, got.Address)
		require.Nil(t, got.CategoryOtherText)
		require.Nil(t, got.TelegramUsername)
		require.Nil(t, got.TelegramFirstName)
		require.Nil(t, got.TelegramLastName)
	})

	t.Run("translates 23505 on creators_iin_unique to ErrCreatorAlreadyExists", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsIINUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorAlreadyExists)
	})

	t.Run("translates 23505 on creators_telegram_user_id_unique to ErrCreatorTelegramAlreadyTaken", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsTelegramUserIDUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorTelegramAlreadyTaken)
	})

	t.Run("translates 23505 on creators_source_application_id_unique to ErrCreatorApplicationNotApprovable", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsSourceApplicationIDUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationNotApprovable)
	})

	t.Run("propagates unrelated 23505 violations as-is", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_idx"})

		_, err := repo.Create(context.Background(), rowFull)
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorAlreadyExists)
		require.NotErrorIs(t, err, domain.ErrCreatorTelegramAlreadyTaken)
		require.NotErrorIs(t, err, domain.ErrCreatorApplicationNotApprovable)
	})

	t.Run("propagates non-unique pg errors with context", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: "creators_source_application_id_fkey", Message: "FK violation"})

		_, err := repo.Create(context.Background(), rowFull)
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorAlreadyExists)
	})

	t.Run("propagates generic db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(errors.New("connection refused"))

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorContains(t, err, "connection refused")
	})
}

func TestCreatorRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, source_application_id, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username, updated_at FROM creators WHERE id = $1"

	birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("ASMR"), "almaty", created, "Айдана", "creator-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "app-1", pointer.ToString("Aidana"), pointer.ToString("M."), int64(9000000001), pointer.ToString("aidana"), created))

		got, err := repo.GetByID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorRow{
			ID:                  "creator-1",
			IIN:                 "950515312348",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			MiddleName:          pointer.ToString("Ивановна"),
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			Address:             pointer.ToString("ул. Абая 1"),
			CategoryOtherText:   pointer.ToString("ASMR"),
			TelegramUserID:      9000000001,
			TelegramUsername:    pointer.ToString("aidana"),
			TelegramFirstName:   pointer.ToString("Aidana"),
			TelegramLastName:    pointer.ToString("M."),
			SourceApplicationID: "app-1",
			CreatedAt:           created,
			UpdatedAt:           created,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByID(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM creators WHERE id = $1"

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("creator-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		require.NoError(t, repo.DeleteForTests(context.Background(), "creator-1"))
	})

	t.Run("missing returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		err := repo.DeleteForTests(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}
