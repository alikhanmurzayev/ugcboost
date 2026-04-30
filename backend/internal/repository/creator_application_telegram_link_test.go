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

func TestCreatorApplicationTelegramLinkRepository_Insert(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_telegram_links " +
		"(application_id,linked_at,telegram_first_name,telegram_last_name,telegram_user_id,telegram_username) " +
		"VALUES ($1,$2,$3,$4,$5,$6) " +
		"RETURNING application_id, linked_at, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username"

	username := "aidana_tg"
	first := "Aidana"
	last := "M."
	linkedAt := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	row := CreatorApplicationTelegramLinkRow{
		ApplicationID:     "app-1",
		TelegramUserID:    int64(123456),
		TelegramUsername:  &username,
		TelegramFirstName: &first,
		TelegramLastName:  &last,
		LinkedAt:          linkedAt,
	}

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, first, last, int64(123456), username).
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}).
				AddRow("app-1", linkedAt, &first, &last, int64(123456), &username))

		got, err := repo.Insert(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &row, got)
	})

	t.Run("PK conflict translates to domain sentinel", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, first, last, int64(123456), username).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorApplicationTelegramLinksPK})

		_, err := repo.Insert(context.Background(), row)
		require.ErrorIs(t, err, domain.ErrTelegramApplicationLinkConflict)
	})

	t.Run("23505 with unknown constraint propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, first, last, int64(123456), username).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_index"})

		_, err := repo.Insert(context.Background(), row)
		require.NotErrorIs(t, err, domain.ErrTelegramApplicationLinkConflict)
		require.Error(t, err)
	})

	t.Run("FK violation 23503 translates to domain.ErrNotFound", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, first, last, int64(123456), username).
			WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: CreatorApplicationTelegramLinksApplicationFK})

		_, err := repo.Insert(context.Background(), row)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("non-pg error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, first, last, int64(123456), username).
			WillReturnError(errors.New("db down"))

		_, err := repo.Insert(context.Background(), row)
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorApplicationTelegramLinkRepository_GetByApplicationID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT application_id, linked_at, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username " +
		"FROM creator_application_telegram_links WHERE application_id = $1"

	username := "aidana_tg"
	linkedAt := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}).
				AddRow("app-1", linkedAt, (*string)(nil), (*string)(nil), int64(42), &username))

		got, err := repo.GetByApplicationID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationTelegramLinkRow{
			ApplicationID:    "app-1",
			TelegramUserID:   int64(42),
			TelegramUsername: &username,
			LinkedAt:         linkedAt,
		}, got)
	})

	t.Run("not found returns wrapped sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}))

		_, err := repo.GetByApplicationID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByApplicationID(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}
