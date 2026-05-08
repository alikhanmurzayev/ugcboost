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
	creatorSelectColumns  = sortColumns(stom.MustNewStom(CreatorRow{}).SetTag(string(tagSelect)).TagValues())
	creatorInsertMapper   = stom.MustNewStom(CreatorRow{}).SetTag(string(tagInsert))
	creatorListRowColumns = sortColumns(stom.MustNewStom(CreatorListRow{}).SetTag(string(tagSelect)).TagValues())
	creatorListProjection = aliasedColumns(creatorListAlias, creatorListRowColumns)
)

// Aliases for the multi-table list query — composing column references via
// constants keeps "no string literals for column names" from the constants
// standard intact.
const (
	creatorListAlias         = "cr"
	creatorListCityAlias     = "ct"
	creatorListCategoryAlias = "ccat"
	creatorListSocialAlias   = "csoc"
)

// CreatorRepo lists every public method of the creator repository.
type CreatorRepo interface {
	Create(ctx context.Context, row CreatorRow) (*CreatorRow, error)
	GetByID(ctx context.Context, id string) (*CreatorRow, error)
	GetTelegramUserIDsByIDs(ctx context.Context, ids []string) (map[string]int64, error)
	List(ctx context.Context, params CreatorListParams) ([]*CreatorListRow, int64, error)
	DeleteForTests(ctx context.Context, id string) error
}

// CreatorListParams carries the validated filter/sort/search/pagination inputs
// the service hands to the repo. The repo trusts these values (sort/order
// whitelisted, page/perPage bounded, ids count capped) and builds the SQL
// query directly — without re-validation.
type CreatorListParams struct {
	IDs        []string
	Cities     []string
	Categories []string
	DateFrom   *time.Time
	DateTo     *time.Time
	AgeFrom    *int
	AgeTo      *int
	Search     string
	Sort       string
	Order      string
	Page       int
	PerPage    int
}

// CreatorListRow is the projected row returned by List. It carries only the
// columns surfaced in the list response — address, category_other_text, the
// telegram_user_id/first/last metadata and source_application_id stay in
// CreatorRow for the GET aggregate. The dedicated row type keeps the SELECT
// explicit at the call site and avoids polluting CreatorRow with a list-only
// projection.
type CreatorListRow struct {
	ID               string    `db:"id"`
	LastName         string    `db:"last_name"`
	FirstName        string    `db:"first_name"`
	MiddleName       *string   `db:"middle_name"`
	IIN              string    `db:"iin"`
	BirthDate        time.Time `db:"birth_date"`
	Phone            string    `db:"phone"`
	CityCode         string    `db:"city_code"`
	TelegramUsername *string   `db:"telegram_username"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
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

// GetTelegramUserIDsByIDs returns a creator-id → telegram_user_id map for
// the requested ids. Missing ids are simply absent from the result; the
// caller compares against the input batch to decide. Empty input yields
// an empty map without hitting the DB.
func (r *creatorRepository) GetTelegramUserIDsByIDs(ctx context.Context, ids []string) (map[string]int64, error) {
	if len(ids) == 0 {
		return map[string]int64{}, nil
	}
	q := sq.Select(CreatorColumnID, CreatorColumnTelegramUserID).
		From(TableCreators).
		Where(sq.Eq{CreatorColumnID: ids})
	type row struct {
		ID             string `db:"id"`
		TelegramUserID int64  `db:"telegram_user_id"`
	}
	rows, err := dbutil.Many[row](ctx, r.db, q)
	if err != nil {
		return nil, fmt.Errorf("creator_repository.GetTelegramUserIDsByIDs: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.ID] = r.TelegramUserID
	}
	return out, nil
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

// List returns one page of approved creators matching the filter set, plus
// the unpaginated total. Both queries share the same WHERE chain so total
// stays in sync with what the page would yield without LIMIT/OFFSET. The
// page query additionally LEFT JOINs cities so sort=city_name resolves the
// human-readable name from the dictionary; that join is added only when the
// sort actually needs it so a scan over an unrelated sort key does not pull
// every row through an extra dictionary lookup. Deactivated city codes leave
// a NULL ct.name; Postgres orders them at the natural extreme of the chosen
// direction (NULLS LAST on ASC / NULLS FIRST on DESC).
//
// Defensive bounds check on Page/PerPage: the handler already validates the
// range, but a future re-caller (cron, another service, a unit test calling
// the repo directly) could pass garbage. A negative `int` cast to `uint64`
// becomes a giant unsigned number — Postgres accepts the OFFSET and runs a
// full table scan to seek that far before returning zero rows.
func (r *creatorRepository) List(ctx context.Context, params CreatorListParams) ([]*CreatorListRow, int64, error) {
	if params.Page < 1 || params.PerPage < 1 {
		return nil, 0, fmt.Errorf("creator_repository.List: invalid pagination page=%d perPage=%d", params.Page, params.PerPage)
	}

	countQ := applyCreatorListFilters(
		sq.Select("COUNT(*)").From(TableCreators+" "+creatorListAlias),
		params,
	)
	total, err := dbutil.Val[int64](ctx, r.db, countQ)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	q := sq.Select(creatorListProjection...).
		From(TableCreators + " " + creatorListAlias)
	if params.Sort == domain.CreatorSortCityName {
		q = q.LeftJoin(creatorListCityJoin())
	}
	q = applyCreatorListFilters(q, params)
	q, err = applyCreatorListOrder(q, params.Sort, params.Order)
	if err != nil {
		return nil, 0, err
	}

	offset := uint64(params.Page-1) * uint64(params.PerPage)
	q = q.Limit(uint64(params.PerPage)).Offset(offset)

	rows, err := dbutil.Many[CreatorListRow](ctx, r.db, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func creatorListCityJoin() string {
	return TableCities + " " + creatorListCityAlias +
		" ON " + creatorListCityAlias + "." + DictionaryColumnCode +
		" = " + creatorListAlias + "." + CreatorColumnCityCode
}

// applyCreatorListFilters appends every active filter condition. Multi-value
// arrays produce IN-clauses (any-of), the categories filter goes through an
// EXISTS subquery on the M:N table, and the search clause spans the six PII
// columns (last/first/middle name, IIN, phone, telegram_username) plus an
// EXISTS on socials.handle. Search is escaped for ILIKE wildcards so an admin
// searching for "100%" or "_test" gets a literal substring match instead of
// Postgres' wildcard semantics. Subqueries are passed to squirrel as Sqlizer
// objects so argument numbering stays correct.
func applyCreatorListFilters(q sq.SelectBuilder, p CreatorListParams) sq.SelectBuilder {
	crCity := creatorListAlias + "." + CreatorColumnCityCode
	crCreatedAt := creatorListAlias + "." + CreatorColumnCreatedAt
	crBirthDate := creatorListAlias + "." + CreatorColumnBirthDate
	crID := creatorListAlias + "." + CreatorColumnID

	if len(p.IDs) > 0 {
		q = q.Where(sq.Eq{crID: p.IDs})
	}
	if len(p.Cities) > 0 {
		q = q.Where(sq.Eq{crCity: p.Cities})
	}
	if len(p.Categories) > 0 {
		ccatCreatorID := creatorListCategoryAlias + "." + CreatorCategoryColumnCreatorID
		ccatCode := creatorListCategoryAlias + "." + CreatorCategoryColumnCategoryCode
		sub := sq.Select("1").
			From(TableCreatorCategories + " " + creatorListCategoryAlias).
			// Cross-column equality wrapped in sq.Expr so the raw SQL fragment
			// is documented as deliberate. Both sides are package-level column
			// constants; user input never enters this string.
			Where(sq.Expr(ccatCreatorID + " = " + crID)).
			Where(sq.Eq{ccatCode: p.Categories})
		q = q.Where(sq.Expr("EXISTS (?)", sub))
	}
	if p.DateFrom != nil {
		q = q.Where(sq.GtOrEq{crCreatedAt: *p.DateFrom})
	}
	if p.DateTo != nil {
		q = q.Where(sq.LtOrEq{crCreatedAt: *p.DateTo})
	}
	if p.AgeFrom != nil {
		q = q.Where(sq.Expr(crBirthDate+" <= NOW()::date - make_interval(years => ?)", *p.AgeFrom))
	}
	if p.AgeTo != nil {
		q = q.Where(sq.Expr(crBirthDate+" > NOW()::date - make_interval(years => ?)", *p.AgeTo+1))
	}
	if p.Search != "" {
		crLastName := creatorListAlias + "." + CreatorColumnLastName
		crFirstName := creatorListAlias + "." + CreatorColumnFirstName
		crMiddleName := creatorListAlias + "." + CreatorColumnMiddleName
		crIIN := creatorListAlias + "." + CreatorColumnIIN
		crPhone := creatorListAlias + "." + CreatorColumnPhone
		crTelegramUsername := creatorListAlias + "." + CreatorColumnTelegramUsername
		csocCreatorID := creatorListSocialAlias + "." + CreatorSocialColumnCreatorID
		csocHandle := creatorListSocialAlias + "." + CreatorSocialColumnHandle
		pattern := "%" + escapeLikeWildcards(p.Search) + "%"
		socialsSub := sq.Select("1").
			From(TableCreatorSocials + " " + creatorListSocialAlias).
			Where(sq.Expr(csocCreatorID + " = " + crID)).
			Where(sq.Expr(csocHandle+` ILIKE ? ESCAPE '\'`, pattern))
		q = q.Where(sq.Or{
			sq.Expr(crLastName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(crFirstName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(crMiddleName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(crIIN+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(crPhone+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(crTelegramUsername+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr("EXISTS (?)", socialsSub),
		})
	}
	return q
}

// applyCreatorListOrder picks the SQL ORDER BY clause for the validated sort
// field. Every branch tail-orders by id ASC so creators with equal sort keys
// stay deterministically ordered across pages and across direction flips —
// fixing the tie-breaker direction independently of the main sort means a
// page boundary captured at sort=DESC stays in the same place when the same
// query is later run with sort=ASC.
//
// Unknown sort returns a wrapped error rather than a silent fallback: the
// service+handler reject unknown sort upstream, so reaching this branch means
// a code-level drift, and silently sorting by created_at would mask the bug.
func applyCreatorListOrder(q sq.SelectBuilder, sort, order string) (sq.SelectBuilder, error) {
	dir := "ASC"
	if order == domain.SortOrderDesc {
		dir = "DESC"
	}
	tieBreaker := creatorListAlias + "." + CreatorColumnID + " ASC"
	switch sort {
	case domain.CreatorSortCreatedAt:
		col := creatorListAlias + "." + CreatorColumnCreatedAt
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorSortUpdatedAt:
		col := creatorListAlias + "." + CreatorColumnUpdatedAt
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorSortBirthDate:
		col := creatorListAlias + "." + CreatorColumnBirthDate
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorSortFullName:
		last := creatorListAlias + "." + CreatorColumnLastName
		first := creatorListAlias + "." + CreatorColumnFirstName
		middle := creatorListAlias + "." + CreatorColumnMiddleName
		return q.OrderBy(last+" "+dir, first+" "+dir, middle+" "+dir, tieBreaker), nil
	case domain.CreatorSortCityName:
		name := creatorListCityAlias + "." + DictionaryColumnName
		return q.OrderBy(name+" "+dir, tieBreaker), nil
	default:
		return q, fmt.Errorf("creator_repository.applyCreatorListOrder: unsupported sort %q", sort)
	}
}
