package repository

import (
	"context"
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

func TestCreatorApplicationTelegramLinkRepository_Insert(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_application_telegram_links (application_id,linked_at,telegram_first_name,telegram_last_name,telegram_user_id,telegram_username) VALUES ($1,$2,$3,$4,$5,$6) RETURNING application_id, linked_at, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username"

	linkedAt := time.Date(2026, 4, 29, 22, 0, 0, 0, time.UTC)

	t.Run("success returns persisted row with metadata", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1", linkedAt, "Айдана", "Муратова", int64(7000123), "test_42").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}).
				AddRow("app-1", linkedAt, pointer.ToString("Айдана"), pointer.ToString("Муратова"), int64(7000123), pointer.ToString("test_42")))

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:     "app-1",
			TelegramUserID:    7000123,
			TelegramUsername:  pointer.ToString("test_42"),
			TelegramFirstName: pointer.ToString("Айдана"),
			TelegramLastName:  pointer.ToString("Муратова"),
			LinkedAt:          linkedAt,
		}
		got, err := repo.Insert(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &row, got)
	})

	t.Run("success with all metadata fields nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-2", linkedAt, nil, nil, int64(7000124), nil).
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}).
				AddRow("app-2", linkedAt, (*string)(nil), (*string)(nil), int64(7000124), (*string)(nil)))

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:  "app-2",
			TelegramUserID: 7000124,
			LinkedAt:       linkedAt,
		}
		got, err := repo.Insert(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &row, got)
	})

	t.Run("UNIQUE on telegram_user_id maps to ErrTelegramAccountLinkConflict", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-3", linkedAt, nil, nil, int64(7000125), nil).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorApplicationTelegramLinksTelegramUserIDKey})

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:  "app-3",
			TelegramUserID: 7000125,
			LinkedAt:       linkedAt,
		}
		_, err := repo.Insert(context.Background(), row)
		require.ErrorIs(t, err, domain.ErrTelegramAccountLinkConflict)
	})

	t.Run("PK conflict on application_id maps to ErrTelegramApplicationLinkConflict", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-4", linkedAt, nil, nil, int64(7000126), nil).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorApplicationTelegramLinksPK})

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:  "app-4",
			TelegramUserID: 7000126,
			LinkedAt:       linkedAt,
		}
		_, err := repo.Insert(context.Background(), row)
		require.ErrorIs(t, err, domain.ErrTelegramApplicationLinkConflict)
	})

	t.Run("23505 with unknown constraint propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-5", linkedAt, nil, nil, int64(7000127), nil).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_constraint"})

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:  "app-5",
			TelegramUserID: 7000127,
			LinkedAt:       linkedAt,
		}
		_, err := repo.Insert(context.Background(), row)
		require.NotErrorIs(t, err, domain.ErrTelegramAccountLinkConflict)
		require.NotErrorIs(t, err, domain.ErrTelegramApplicationLinkConflict)
	})

	t.Run("generic error propagates with context", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-6", linkedAt, nil, nil, int64(7000128), nil).
			WillReturnError(errors.New("connection refused"))

		row := CreatorApplicationTelegramLinkRow{
			ApplicationID:  "app-6",
			TelegramUserID: 7000128,
			LinkedAt:       linkedAt,
		}
		_, err := repo.Insert(context.Background(), row)
		require.ErrorContains(t, err, "connection refused")
	})
}

func TestCreatorApplicationTelegramLinkRepository_GetByApplicationID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT application_id, linked_at, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username FROM creator_application_telegram_links WHERE application_id = $1"

	linkedAt := time.Date(2026, 4, 29, 22, 5, 0, 0, time.UTC)

	t.Run("found returns row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"application_id", "linked_at", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username"}).
				AddRow("app-1", linkedAt, pointer.ToString("Айдана"), pointer.ToString("Муратова"), int64(7000123), pointer.ToString("test_42")))

		got, err := repo.GetByApplicationID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationTelegramLinkRow{
			ApplicationID:     "app-1",
			TelegramUserID:    7000123,
			TelegramUsername:  pointer.ToString("test_42"),
			TelegramFirstName: pointer.ToString("Айдана"),
			TelegramLastName:  pointer.ToString("Муратова"),
			LinkedAt:          linkedAt,
		}, got)
	})

	t.Run("not found returns sql.ErrNoRows (via pgx.ErrNoRows)", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationTelegramLinkRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByApplicationID(context.Background(), "missing")
		require.Error(t, err)
		require.ErrorContains(t, err, pgx.ErrNoRows.Error())
	})

	t.Run("propagates other errors", func(t *testing.T) {
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
