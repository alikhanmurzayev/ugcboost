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
const (
	TableCreatorApplicationTelegramLinks = "creator_application_telegram_links"

	CreatorApplicationTelegramLinkColumnApplicationID     = "application_id"
	CreatorApplicationTelegramLinkColumnTelegramUserID    = "telegram_user_id"
	CreatorApplicationTelegramLinkColumnTelegramUsername  = "telegram_username"
	CreatorApplicationTelegramLinkColumnTelegramFirstName = "telegram_first_name"
	CreatorApplicationTelegramLinkColumnTelegramLastName  = "telegram_last_name"
	CreatorApplicationTelegramLinkColumnLinkedAt          = "linked_at"

	// Used to disambiguate 23505 violations from any future unique index.
	CreatorApplicationTelegramLinksPK = "creator_application_telegram_links_pkey"
	// Used to disambiguate 23503 violations from any future foreign key.
	CreatorApplicationTelegramLinksApplicationFK = "creator_application_telegram_links_application_id_fkey"
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

// CreatorApplicationTelegramLinkRepo is the public interface of the link repo.
type CreatorApplicationTelegramLinkRepo interface {
	Insert(ctx context.Context, row CreatorApplicationTelegramLinkRow) (*CreatorApplicationTelegramLinkRow, error)
	GetByApplicationID(ctx context.Context, applicationID string) (*CreatorApplicationTelegramLinkRow, error)
}

type creatorApplicationTelegramLinkRepository struct {
	db dbutil.DB
}

// Insert creates a link row. A PK conflict on application_id (concurrent
// /start for the same application) is translated to
// ErrTelegramApplicationLinkConflict so the service can re-read and decide
// idempotent vs business error. An FK violation on application_id (parent
// row gone between the service's preflight and our insert) is translated
// to domain.ErrNotFound — the service maps it to MessageApplicationNotFound.
func (r *creatorApplicationTelegramLinkRepository) Insert(ctx context.Context, row CreatorApplicationTelegramLinkRow) (*CreatorApplicationTelegramLinkRow, error) {
	q := sq.Insert(TableCreatorApplicationTelegramLinks).
		SetMap(toMap(row, creatorApplicationTelegramLinkInsertMapper)).
		Suffix(returningClause(creatorApplicationTelegramLinkSelectColumns))
	result, err := dbutil.One[CreatorApplicationTelegramLinkRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23505" && pgErr.ConstraintName == CreatorApplicationTelegramLinksPK:
				return nil, domain.ErrTelegramApplicationLinkConflict
			case pgErr.Code == "23503" && pgErr.ConstraintName == CreatorApplicationTelegramLinksApplicationFK:
				return nil, domain.ErrNotFound
			}
		}
		return nil, err
	}
	return result, nil
}

// GetByApplicationID propagates wrapped sql.ErrNoRows when no link exists —
// the service interprets that as "not linked yet".
func (r *creatorApplicationTelegramLinkRepository) GetByApplicationID(ctx context.Context, applicationID string) (*CreatorApplicationTelegramLinkRow, error) {
	q := sq.Select(creatorApplicationTelegramLinkSelectColumns...).
		From(TableCreatorApplicationTelegramLinks).
		Where(sq.Eq{CreatorApplicationTelegramLinkColumnApplicationID: applicationID})
	return dbutil.One[CreatorApplicationTelegramLinkRow](ctx, r.db, q)
}
