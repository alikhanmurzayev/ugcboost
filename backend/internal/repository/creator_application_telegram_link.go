package repository

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Creator application Telegram link table, columns and constraint identifiers.
//
// The constraint names mirror what Postgres assigns by default for the schema
// shipped in 20260429224431_creator_application_telegram_links.sql:
//
//	PRIMARY KEY (application_id) → creator_application_telegram_links_pkey
//	UNIQUE      (telegram_user_id) → creator_application_telegram_links_telegram_user_id_key
//
// They are kept as constants so the repo can map a 23505 Postgres error to the
// right domain sentinel without string-fishing in error messages.
const (
	TableCreatorApplicationTelegramLinks = "creator_application_telegram_links"

	CreatorApplicationTelegramLinkColumnApplicationID     = "application_id"
	CreatorApplicationTelegramLinkColumnTelegramUserID    = "telegram_user_id"
	CreatorApplicationTelegramLinkColumnTelegramUsername  = "telegram_username"
	CreatorApplicationTelegramLinkColumnTelegramFirstName = "telegram_first_name"
	CreatorApplicationTelegramLinkColumnTelegramLastName  = "telegram_last_name"
	CreatorApplicationTelegramLinkColumnLinkedAt          = "linked_at"

	CreatorApplicationTelegramLinksPK                = "creator_application_telegram_links_pkey"
	CreatorApplicationTelegramLinksTelegramUserIDKey = "creator_application_telegram_links_telegram_user_id_key"
)

// CreatorApplicationTelegramLinkRow maps to creator_application_telegram_links.
// All five business columns carry an insert tag — the service stamps linked_at
// explicitly so it matches the audit_logs row written in the same transaction.
type CreatorApplicationTelegramLinkRow struct {
	ApplicationID     string    `db:"application_id"      insert:"application_id"`
	TelegramUserID    int64     `db:"telegram_user_id"    insert:"telegram_user_id"`
	TelegramUsername  *string   `db:"telegram_username"   insert:"telegram_username"`
	TelegramFirstName *string   `db:"telegram_first_name" insert:"telegram_first_name"`
	TelegramLastName  *string   `db:"telegram_last_name"  insert:"telegram_last_name"`
	LinkedAt          time.Time `db:"linked_at"           insert:"linked_at"`
}

var (
	creatorApplicationTelegramLinkSelectColumns = sortColumns(stom.MustNewStom(CreatorApplicationTelegramLinkRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationTelegramLinkInsertMapper  = stom.MustNewStom(CreatorApplicationTelegramLinkRow{}).SetTag(string(tagInsert))
)

// CreatorApplicationTelegramLinkRepo lists every public method of the link
// repository. Insert translates SQLSTATE 23505 violations into domain
// sentinels so the service can answer with a business error instead of a 500.
type CreatorApplicationTelegramLinkRepo interface {
	Insert(ctx context.Context, row CreatorApplicationTelegramLinkRow) (*CreatorApplicationTelegramLinkRow, error)
	GetByApplicationID(ctx context.Context, applicationID string) (*CreatorApplicationTelegramLinkRow, error)
}

type creatorApplicationTelegramLinkRepository struct {
	db dbutil.DB
}

// Insert creates a new link row and returns the persisted record. Two race
// conditions are translated to domain sentinels:
//
//   - PRIMARY KEY conflict on application_id → ErrTelegramApplicationLinkConflict
//     (another /start beat us; service re-reads to decide idempotent vs error).
//   - UNIQUE conflict on telegram_user_id → ErrTelegramAccountLinkConflict
//     (this Telegram account is already bound to another application).
func (r *creatorApplicationTelegramLinkRepository) Insert(ctx context.Context, row CreatorApplicationTelegramLinkRow) (*CreatorApplicationTelegramLinkRow, error) {
	q := sq.Insert(TableCreatorApplicationTelegramLinks).
		SetMap(toMap(row, creatorApplicationTelegramLinkInsertMapper)).
		Suffix(returningClause(creatorApplicationTelegramLinkSelectColumns))
	result, err := dbutil.One[CreatorApplicationTelegramLinkRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case CreatorApplicationTelegramLinksTelegramUserIDKey:
				return nil, domain.ErrTelegramAccountLinkConflict
			case CreatorApplicationTelegramLinksPK:
				return nil, domain.ErrTelegramApplicationLinkConflict
			}
		}
		return nil, err
	}
	return result, nil
}

// GetByApplicationID returns the link row for the given application id.
// dbutil.One propagates sql.ErrNoRows (wrapped) when the row is missing —
// the service interprets that as "not linked yet" and continues to INSERT.
func (r *creatorApplicationTelegramLinkRepository) GetByApplicationID(ctx context.Context, applicationID string) (*CreatorApplicationTelegramLinkRow, error) {
	q := sq.Select(creatorApplicationTelegramLinkSelectColumns...).
		From(TableCreatorApplicationTelegramLinks).
		Where(sq.Eq{CreatorApplicationTelegramLinkColumnApplicationID: applicationID})
	return dbutil.One[CreatorApplicationTelegramLinkRow](ctx, r.db, q)
}
