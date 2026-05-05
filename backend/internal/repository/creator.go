package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Creator unique constraint names. Matched against pgErr.ConstraintName so
// the 23505-switch translates concurrent races into precise domain sentinels
// instead of leaking the raw Postgres error.
const (
	CreatorsIINUnique                 = "creators_iin_unique"
	CreatorsTelegramUserIDUnique      = "creators_telegram_user_id_unique"
	CreatorsSourceApplicationIDUnique = "creators_source_application_id_unique"
)

// Creators table and column names.
const (
	TableCreators                  = "creators"
	CreatorColumnID                = "id"
	CreatorColumnIIN               = "iin"
	CreatorColumnLastName          = "last_name"
	CreatorColumnFirstName         = "first_name"
	CreatorColumnMiddleName        = "middle_name"
	CreatorColumnBirthDate         = "birth_date"
	CreatorColumnPhone             = "phone"
	CreatorColumnCityCode          = "city_code"
	CreatorColumnAddress           = "address"
	CreatorColumnCategoryOtherText = "category_other_text"
	CreatorColumnTelegramUserID    = "telegram_user_id"
	CreatorColumnTelegramUsername  = "telegram_username"
	CreatorColumnTelegramFirstName = "telegram_first_name"
	CreatorColumnTelegramLastName  = "telegram_last_name"
	CreatorColumnSourceAppID       = "source_application_id"
	CreatorColumnCreatedAt         = "created_at"
	CreatorColumnUpdatedAt         = "updated_at"
)

// CreatorRow maps to the creators table. The row is the flat snapshot of an
// approved CreatorApplication (PII + Telegram metadata + back-link to the
// originating application). The repo treats the input as trusted — service
// layer (18b) is responsible for validating the source application before
// inserting.
type CreatorRow struct {
	ID                  string    `db:"id"`
	IIN                 string    `db:"iin"                   insert:"iin"`
	LastName            string    `db:"last_name"             insert:"last_name"`
	FirstName           string    `db:"first_name"            insert:"first_name"`
	MiddleName          *string   `db:"middle_name"           insert:"middle_name"`
	BirthDate           time.Time `db:"birth_date"            insert:"birth_date"`
	Phone               string    `db:"phone"                 insert:"phone"`
	CityCode            string    `db:"city_code"             insert:"city_code"`
	Address             *string   `db:"address"               insert:"address"`
	CategoryOtherText   *string   `db:"category_other_text"   insert:"category_other_text"`
	TelegramUserID      int64     `db:"telegram_user_id"      insert:"telegram_user_id"`
	TelegramUsername    *string   `db:"telegram_username"     insert:"telegram_username"`
	TelegramFirstName   *string   `db:"telegram_first_name"   insert:"telegram_first_name"`
	TelegramLastName    *string   `db:"telegram_last_name"    insert:"telegram_last_name"`
	SourceApplicationID string    `db:"source_application_id" insert:"source_application_id"`
	CreatedAt           time.Time `db:"created_at"`
	UpdatedAt           time.Time `db:"updated_at"`
}

var (
	creatorSelectColumns = sortColumns(stom.MustNewStom(CreatorRow{}).SetTag(string(tagSelect)).TagValues())
	creatorInsertMapper  = stom.MustNewStom(CreatorRow{}).SetTag(string(tagInsert))
)

// CreatorRepo lists every public method of the creator repository.
type CreatorRepo interface {
	Create(ctx context.Context, row CreatorRow) (*CreatorRow, error)
	GetByID(ctx context.Context, id string) (*CreatorRow, error)
	DeleteForTests(ctx context.Context, id string) error
}

type creatorRepository struct {
	db dbutil.DB
}

// Create inserts a new creator row and returns the persisted row with
// DB-generated fields (id, created_at, updated_at) populated.
//
// Three unique constraints can fire SQLSTATE 23505 here, each surfaced as a
// distinct domain sentinel so the service layer (18b) can map them into
// precise user-facing codes. Unrelated constraint violations are propagated
// as wrapped errors. Postgres reports whichever unique index it checked
// first (oid order in the system catalog), not necessarily the one most
// relevant to the caller; the service layer takes a row-level lock on the
// source application *before* this INSERT to keep the concurrent-approve
// race deterministic ("two approves on the same application → exactly one
// NotApprovable") rather than relying on which index Postgres surfaces.
func (r *creatorRepository) Create(ctx context.Context, row CreatorRow) (*CreatorRow, error) {
	q := sq.Insert(TableCreators).
		SetMap(toMap(row, creatorInsertMapper)).
		Suffix(returningClause(creatorSelectColumns))
	result, err := dbutil.One[CreatorRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case CreatorsIINUnique:
				return nil, domain.ErrCreatorAlreadyExists
			case CreatorsTelegramUserIDUnique:
				return nil, domain.ErrCreatorTelegramAlreadyTaken
			case CreatorsSourceApplicationIDUnique:
				return nil, domain.ErrCreatorApplicationNotApprovable
			}
		}
		return nil, err
	}
	return result, nil
}

// GetByID returns the creator row by id. dbutil.One propagates the
// underlying sql.ErrNoRows wrapped — the service forwards it untouched so
// the handler (18c) maps it to a 404 via errors.Is.
func (r *creatorRepository) GetByID(ctx context.Context, id string) (*CreatorRow, error) {
	q := sq.Select(creatorSelectColumns...).
		From(TableCreators).
		Where(sq.Eq{CreatorColumnID: id})
	return dbutil.One[CreatorRow](ctx, r.db, q)
}

// DeleteForTests hard-deletes a creator by id. Children in
// creator_socials and creator_categories cascade automatically. Returns
// sql.ErrNoRows when the creator does not exist, matching the cleanup-stack
// semantics where "already gone" is treated as success.
func (r *creatorRepository) DeleteForTests(ctx context.Context, id string) error {
	q := sq.Delete(TableCreators).Where(sq.Eq{CreatorColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
