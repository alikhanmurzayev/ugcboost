package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Table and column constants.
const (
	TableTelegramMessages = "telegram_messages"

	TelegramMessageColumnID                = "id"
	TelegramMessageColumnChatID            = "chat_id"
	TelegramMessageColumnDirection         = "direction"
	TelegramMessageColumnText              = "text"
	TelegramMessageColumnTelegramMessageID = "telegram_message_id"
	TelegramMessageColumnTelegramUsername  = "telegram_username"
	TelegramMessageColumnStatus            = "status"
	TelegramMessageColumnError             = "error"
	TelegramMessageColumnCreatedAt         = "created_at"
)

// TelegramMessagesInboundDedupUnique is the partial UNIQUE index name. EAFP
// catch translates SQLSTATE 23505 on this constraint into
// domain.ErrTelegramMessageAlreadyRecorded.
const TelegramMessagesInboundDedupUnique = "telegram_messages_inbound_dedup_unique"

// TelegramMessageRow maps to the telegram_messages table. status/error stay
// pointers because they are NULL for inbound rows; telegram_message_id is
// nullable for outbound failures (no Telegram message_id returned).
type TelegramMessageRow struct {
	ID                string    `db:"id"`
	ChatID            int64     `db:"chat_id"             insert:"chat_id"`
	Direction         string    `db:"direction"           insert:"direction"`
	Text              string    `db:"text"                insert:"text"`
	TelegramMessageID *int64    `db:"telegram_message_id" insert:"telegram_message_id"`
	TelegramUsername  *string   `db:"telegram_username"   insert:"telegram_username"`
	Status            *string   `db:"status"              insert:"status"`
	Error             *string   `db:"error"               insert:"error"`
	CreatedAt         time.Time `db:"created_at"`
}

var (
	telegramMessageSelectColumns = sortColumns(stom.MustNewStom(TelegramMessageRow{}).SetTag(string(tagSelect)).TagValues())
	telegramMessageInsertMapper  = stom.MustNewStom(TelegramMessageRow{}).SetTag(string(tagInsert))
)

// TelegramMessageRepo lists every public method of the repository.
type TelegramMessageRepo interface {
	Insert(ctx context.Context, row *TelegramMessageRow) error
	ListByChat(ctx context.Context, chatID int64, cursor *domain.TelegramMessagesCursor, limit int) ([]*TelegramMessageRow, error)
	DeleteByChatForTests(ctx context.Context, chatID int64) error
}

type telegramMessageRepository struct {
	db dbutil.DB
}

// Insert writes one row and populates row.ID + row.CreatedAt from the DB.
// Caller passes a heap-allocated row so the RETURNING values flow back; this
// matters for the test seed endpoint that surfaces the new id to the e2e
// client. The 23505-on-inbound-dedup branch returns the domain sentinel; any
// other DB error is wrapped with context.
func (r *telegramMessageRepository) Insert(ctx context.Context, row *TelegramMessageRow) error {
	q := sq.Insert(TableTelegramMessages).
		SetMap(toMap(*row, telegramMessageInsertMapper)).
		Suffix(returningClause([]string{TelegramMessageColumnID, TelegramMessageColumnCreatedAt}))
	type returned struct {
		ID        string    `db:"id"`
		CreatedAt time.Time `db:"created_at"`
	}
	out, err := dbutil.One[returned](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			pgErr.ConstraintName == TelegramMessagesInboundDedupUnique {
			return domain.ErrTelegramMessageAlreadyRecorded
		}
		return fmt.Errorf("telegram_message_repository.Insert: %w", err)
	}
	row.ID = out.ID
	row.CreatedAt = out.CreatedAt
	return nil
}

// ListByChat returns up to `limit` rows for the chat, ordered DESC by
// (created_at, id). The cursor (nil for the first page) narrows the scan to
// rows strictly older than the previous page's last row — the tuple
// comparison `(created_at, id) < (cursorCreatedAt, cursorID)` is exact and
// matches the index direction.
//
// The repo does not implement the limit+1 hasMore trick — that is a service
// concern (it owns the nextCursor encoding). Repo returns exactly the slice
// the service asked for.
func (r *telegramMessageRepository) ListByChat(
	ctx context.Context,
	chatID int64,
	cursor *domain.TelegramMessagesCursor,
	limit int,
) ([]*TelegramMessageRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("telegram_message_repository.ListByChat: invalid limit=%d", limit)
	}
	q := sq.Select(telegramMessageSelectColumns...).
		From(TableTelegramMessages).
		Where(sq.Eq{TelegramMessageColumnChatID: chatID})
	if cursor != nil {
		q = q.Where(
			sq.Expr(
				"("+TelegramMessageColumnCreatedAt+", "+TelegramMessageColumnID+") < (?, ?)",
				cursor.CreatedAt, cursor.ID,
			),
		)
	}
	q = q.OrderBy(
		TelegramMessageColumnCreatedAt+" DESC",
		TelegramMessageColumnID+" DESC",
	).Limit(uint64(limit))
	rows, err := dbutil.Many[TelegramMessageRow](ctx, r.db, q)
	if err != nil {
		return nil, fmt.Errorf("telegram_message_repository.ListByChat: %w", err)
	}
	return rows, nil
}

// DeleteByChatForTests hard-deletes every row for the chat. Returns
// sql.ErrNoRows when nothing matched so the testapi handler can map that
// to an idempotent 204 without leaking the count. Cleanup endpoint only.
func (r *telegramMessageRepository) DeleteByChatForTests(ctx context.Context, chatID int64) error {
	q := sq.Delete(TableTelegramMessages).Where(sq.Eq{TelegramMessageColumnChatID: chatID})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
