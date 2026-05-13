package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestTelegramMessageRepository_Insert(t *testing.T) {
	t.Parallel()

	const insertSQL = "INSERT INTO telegram_messages (chat_id,direction,error,status,telegram_message_id,telegram_username,text) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at"

	t.Run("happy inbound row populates id and created_at", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		createdAt := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "inbound", nil, nil, int64(7), "aidana", "hello").
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("row-1", createdAt))

		row := &TelegramMessageRow{
			ChatID:            42,
			Direction:         domain.TelegramMessageDirectionInbound,
			Text:              "hello",
			TelegramMessageID: pointer.ToInt64(7),
			TelegramUsername:  pointer.ToString("aidana"),
		}
		err := repo.Insert(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, "row-1", row.ID)
		require.Equal(t, createdAt, row.CreatedAt)
	})

	t.Run("happy outbound sent: telegram_message_id and status sent", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		createdAt := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "outbound", nil, "sent", int64(99), nil, "hi from bot").
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("row-2", createdAt))

		row := &TelegramMessageRow{
			ChatID:            42,
			Direction:         domain.TelegramMessageDirectionOutbound,
			Text:              "hi from bot",
			TelegramMessageID: pointer.ToInt64(99),
			Status:            pointer.ToString(domain.TelegramMessageStatusSent),
		}
		require.NoError(t, repo.Insert(context.Background(), row))
		require.Equal(t, "row-2", row.ID)
	})

	t.Run("happy outbound failed: error populated, telegram_message_id NULL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		createdAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "outbound", "bot blocked", "failed", nil, nil, "hi from bot").
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow("row-3", createdAt))

		row := &TelegramMessageRow{
			ChatID:    42,
			Direction: domain.TelegramMessageDirectionOutbound,
			Text:      "hi from bot",
			Status:    pointer.ToString(domain.TelegramMessageStatusFailed),
			Error:     pointer.ToString("bot blocked"),
		}
		require.NoError(t, repo.Insert(context.Background(), row))
		require.Equal(t, "row-3", row.ID)
	})

	t.Run("23505 on inbound dedup unique returns domain sentinel", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "inbound", nil, nil, int64(7), nil, "hello").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: TelegramMessagesInboundDedupUnique})

		row := &TelegramMessageRow{
			ChatID:            42,
			Direction:         domain.TelegramMessageDirectionInbound,
			Text:              "hello",
			TelegramMessageID: pointer.ToInt64(7),
		}
		err := repo.Insert(context.Background(), row)
		require.ErrorIs(t, err, domain.ErrTelegramMessageAlreadyRecorded)
	})

	t.Run("23505 on a different constraint wraps as generic error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "inbound", nil, nil, int64(7), nil, "hello").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_unique"})

		row := &TelegramMessageRow{
			ChatID:            42,
			Direction:         domain.TelegramMessageDirectionInbound,
			Text:              "hello",
			TelegramMessageID: pointer.ToInt64(7),
		}
		err := repo.Insert(context.Background(), row)
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrTelegramMessageAlreadyRecorded)
		require.ErrorContains(t, err, "telegram_message_repository.Insert")
	})

	t.Run("generic db error wraps with context", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectQuery(insertSQL).
			WithArgs(int64(42), "inbound", nil, nil, nil, nil, "hello").
			WillReturnError(errors.New("connection refused"))

		row := &TelegramMessageRow{
			ChatID:    42,
			Direction: domain.TelegramMessageDirectionInbound,
			Text:      "hello",
		}
		err := repo.Insert(context.Background(), row)
		require.ErrorContains(t, err, "telegram_message_repository.Insert")
		require.ErrorContains(t, err, "connection refused")
	})
}

func TestTelegramMessageRepository_ListByChat(t *testing.T) {
	t.Parallel()

	const selectCols = "SELECT chat_id, created_at, direction, error, id, status, telegram_message_id, telegram_username, text FROM telegram_messages"
	const noCursorSQL = selectCols + " WHERE chat_id = $1 ORDER BY created_at DESC, id DESC LIMIT 6"
	const withCursorSQL = selectCols + " WHERE chat_id = $1 AND (created_at, id) < ($2, $3) ORDER BY created_at DESC, id DESC LIMIT 6"

	rowsHeader := []string{"chat_id", "created_at", "direction", "error", "id", "status", "telegram_message_id", "telegram_username", "text"}

	t.Run("invalid limit returns error before any SQL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}
		_, err := repo.ListByChat(context.Background(), 42, nil, 0)
		require.ErrorContains(t, err, "invalid limit")
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectQuery(noCursorSQL).
			WithArgs(int64(42)).
			WillReturnRows(pgxmock.NewRows(rowsHeader))

		rows, err := repo.ListByChat(context.Background(), 42, nil, 6)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("first page: no cursor, sorted DESC by created_at,id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		t1 := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		mock.ExpectQuery(noCursorSQL).
			WithArgs(int64(42)).
			WillReturnRows(pgxmock.NewRows(rowsHeader).
				AddRow(int64(42), t1, "inbound", nil, "id-2", nil, pointer.ToInt64(2), pointer.ToString("u"), "second").
				AddRow(int64(42), t2, "outbound", nil, "id-1", pointer.ToString("sent"), pointer.ToInt64(1), nil, "first"))

		rows, err := repo.ListByChat(context.Background(), 42, nil, 6)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.Equal(t, "id-2", rows[0].ID)
		require.Equal(t, "id-1", rows[1].ID)
		require.Equal(t, t1, rows[0].CreatedAt)
	})

	t.Run("with cursor: applies tuple comparison", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		cursorAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery(withCursorSQL).
			WithArgs(int64(42), cursorAt, "id-prev").
			WillReturnRows(pgxmock.NewRows(rowsHeader))

		rows, err := repo.ListByChat(context.Background(), 42, &domain.TelegramMessagesCursor{
			CreatedAt: cursorAt,
			ID:        "id-prev",
		}, 6)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("db error propagates with context", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectQuery(noCursorSQL).
			WithArgs(int64(42)).
			WillReturnError(errors.New("query failed"))

		_, err := repo.ListByChat(context.Background(), 42, nil, 6)
		require.ErrorContains(t, err, "telegram_message_repository.ListByChat")
		require.ErrorContains(t, err, "query failed")
	})
}

func TestTelegramMessageRepository_DeleteByChatForTests(t *testing.T) {
	t.Parallel()

	const deleteSQL = "DELETE FROM telegram_messages WHERE chat_id = $1"

	t.Run("happy: rows removed", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectExec(deleteSQL).
			WithArgs(int64(42)).
			WillReturnResult(pgconn.NewCommandTag("DELETE 3"))

		require.NoError(t, repo.DeleteByChatForTests(context.Background(), 42))
	})

	t.Run("zero rows affected returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectExec(deleteSQL).
			WithArgs(int64(42)).
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteByChatForTests(context.Background(), 42)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("db error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &telegramMessageRepository{db: mock}

		mock.ExpectExec(deleteSQL).
			WithArgs(int64(42)).
			WillReturnError(errors.New("conn closed"))

		err := repo.DeleteByChatForTests(context.Background(), 42)
		require.ErrorContains(t, err, "conn closed")
	})
}
