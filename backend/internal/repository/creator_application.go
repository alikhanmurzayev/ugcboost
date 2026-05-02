package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

const CreatorApplicationsIINActiveIdx = "creator_applications_iin_active_idx"

// Creator applications table and column names.
const (
	TableCreatorApplications                  = "creator_applications"
	CreatorApplicationColumnID                = "id"
	CreatorApplicationColumnLastName          = "last_name"
	CreatorApplicationColumnFirstName         = "first_name"
	CreatorApplicationColumnMiddleName        = "middle_name"
	CreatorApplicationColumnIIN               = "iin"
	CreatorApplicationColumnBirthDate         = "birth_date"
	CreatorApplicationColumnPhone             = "phone"
	CreatorApplicationColumnCityCode          = "city_code"
	CreatorApplicationColumnAddress           = "address"
	CreatorApplicationColumnCategoryOtherText = "category_other_text"
	CreatorApplicationColumnStatus            = "status"
	CreatorApplicationColumnCreatedAt         = "created_at"
	CreatorApplicationColumnUpdatedAt         = "updated_at"
)

// CreatorApplicationRow maps to the creator_applications table. Status is
// passed in by the service layer (no DB DEFAULT after the relax_constraints
// migration), so it carries an insert tag. Future moderation flows update the
// column via dedicated endpoints, not via the standard INSERT path.
type CreatorApplicationRow struct {
	ID                string    `db:"id"`
	LastName          string    `db:"last_name"           insert:"last_name"`
	FirstName         string    `db:"first_name"          insert:"first_name"`
	MiddleName        *string   `db:"middle_name"         insert:"middle_name"`
	IIN               string    `db:"iin"                 insert:"iin"`
	BirthDate         time.Time `db:"birth_date"          insert:"birth_date"`
	Phone             string    `db:"phone"               insert:"phone"`
	CityCode          string    `db:"city_code"           insert:"city_code"`
	Address           *string   `db:"address"             insert:"address"`
	CategoryOtherText *string   `db:"category_other_text" insert:"category_other_text"`
	Status            string    `db:"status"              insert:"status"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

var (
	creatorApplicationSelectColumns = sortColumns(stom.MustNewStom(CreatorApplicationRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationInsertMapper  = stom.MustNewStom(CreatorApplicationRow{}).SetTag(string(tagInsert))
)

// CreatorApplicationRepo lists all public methods of the creator application
// repository.
type CreatorApplicationRepo interface {
	HasActiveByIIN(ctx context.Context, iin string) (bool, error)
	Create(ctx context.Context, row CreatorApplicationRow) (*CreatorApplicationRow, error)
	GetByID(ctx context.Context, id string) (*CreatorApplicationRow, error)
	List(ctx context.Context, params CreatorApplicationListParams) ([]*CreatorApplicationListRow, int64, error)
	DeleteForTests(ctx context.Context, id string) error
}

// Aliases for the multi-table list query — composing column references via
// constants keeps "no string literals for column names" from the constants
// standard intact.
const (
	creatorApplicationListAlias            = "ca"
	creatorApplicationListTelegramAlias    = "tgl"
	creatorApplicationListCityAlias        = "ct"
	creatorApplicationListCategoryAlias    = "cac"
	creatorApplicationListSocialAlias      = "cas"
	creatorApplicationListTelegramLinkedAs = "telegram_linked"
)

// CreatorApplicationListParams carries the validated search/filter/sort/pagination
// inputs that the service hands to the repo. The repo trusts these values
// (sort/order whitelisted, page/perPage bounded) and builds the SQL query
// directly — without re-validation.
type CreatorApplicationListParams struct {
	Statuses       []string
	Cities         []string
	Categories     []string
	DateFrom       *time.Time
	DateTo         *time.Time
	AgeFrom        *int
	AgeTo          *int
	TelegramLinked *bool
	Search         string
	Sort           string
	Order          string
	Page           int
	PerPage        int
}

// CreatorApplicationListRow is the projected row returned by List. It mixes
// columns from creator_applications with a derived telegram_linked boolean
// computed in-query (LEFT JOIN to creator_application_telegram_links). The
// dedicated row type avoids polluting CreatorApplicationRow with a non-table
// field and keeps the SELECT explicit at the call site.
type CreatorApplicationListRow struct {
	ID             string    `db:"id"`
	LastName       string    `db:"last_name"`
	FirstName      string    `db:"first_name"`
	MiddleName     *string   `db:"middle_name"`
	BirthDate      time.Time `db:"birth_date"`
	CityCode       string    `db:"city_code"`
	Status         string    `db:"status"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
	TelegramLinked bool      `db:"telegram_linked"`
}

type creatorApplicationRepository struct {
	db dbutil.DB
}

// HasActiveByIIN reports whether any application with the given IIN is in an
// "active" status (see domain.CreatorApplicationActiveStatuses). Rejected
// applications are ignored so creators can re-apply per FR17.
func (r *creatorApplicationRepository) HasActiveByIIN(ctx context.Context, iin string) (bool, error) {
	q := sq.Select("1").
		From(TableCreatorApplications).
		Where(sq.Eq{CreatorApplicationColumnIIN: iin}).
		Where(sq.Eq{CreatorApplicationColumnStatus: domain.CreatorApplicationActiveStatuses}).
		Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Create inserts a new application row and returns the persisted row with
// DB-generated fields populated (id, status, created_at, updated_at).
// Concurrent submits with the same IIN can pass HasActiveByIIN and race on
// INSERT — the partial unique index fires and pgx returns SQLSTATE 23505. We
// translate that specific case into domain.ErrCreatorApplicationDuplicate so
// the service can still answer 409, not 500.
func (r *creatorApplicationRepository) Create(ctx context.Context, row CreatorApplicationRow) (*CreatorApplicationRow, error) {
	q := sq.Insert(TableCreatorApplications).
		SetMap(toMap(row, creatorApplicationInsertMapper)).
		Suffix(returningClause(creatorApplicationSelectColumns))
	result, err := dbutil.One[CreatorApplicationRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, CreatorApplicationsIINActiveIdx) {
			return nil, domain.ErrCreatorApplicationDuplicate
		}
		return nil, err
	}
	return result, nil
}

// GetByID returns the application row by id. dbutil.One propagates the
// underlying sql.ErrNoRows (wrapped) when the row is missing, which the
// service forwards untouched so the handler can map it to 404 via errors.Is.
func (r *creatorApplicationRepository) GetByID(ctx context.Context, id string) (*CreatorApplicationRow, error) {
	q := sq.Select(creatorApplicationSelectColumns...).
		From(TableCreatorApplications).
		Where(sq.Eq{CreatorApplicationColumnID: id})
	return dbutil.One[CreatorApplicationRow](ctx, r.db, q)
}

// DeleteForTests hard-deletes an application by id. Related rows in
// creator_application_{categories,socials,consents} cascade automatically.
// Returns sql.ErrNoRows when the application does not exist, matching the
// semantics the cleanup stack relies on to treat "already gone" as success.
func (r *creatorApplicationRepository) DeleteForTests(ctx context.Context, id string) error {
	q := sq.Delete(TableCreatorApplications).Where(sq.Eq{CreatorApplicationColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// List returns one page of applications matching the filter set, plus the
// unpaginated total. Both queries share the LEFT JOIN to
// creator_application_telegram_links so the telegramLinked filter and the
// derived projection stay consistent. The page query additionally LEFT JOINs
// cities so sort=city_name resolves the human-readable name from the
// dictionary; that join is added only when the sort actually needs it so a
// scan over an unrelated sort key does not pull every row through an extra
// dictionary lookup. Deactivated city codes leave a NULL ct.name; Postgres
// orders them at the natural extreme of the chosen direction (NULLS LAST on
// ASC / NULLS FIRST on DESC).
//
// Defensive bounds check on Page/PerPage: the handler already validates the
// range, but a future re-caller (cron, another service, a unit test calling
// the repo directly) could pass garbage. Without this guard `int → uint64`
// silently wraps a negative offset into a giant unsigned number and feeds a
// gigabyte-sized OFFSET to Postgres.
func (r *creatorApplicationRepository) List(ctx context.Context, params CreatorApplicationListParams) ([]*CreatorApplicationListRow, int64, error) {
	if params.Page < 1 || params.PerPage < 1 {
		return nil, 0, fmt.Errorf("creator_application_repository.List: invalid pagination page=%d perPage=%d", params.Page, params.PerPage)
	}

	countQ := sq.Select("COUNT(*)").
		From(TableCreatorApplications + " " + creatorApplicationListAlias).
		LeftJoin(creatorApplicationListTelegramJoin())
	countQ, err := applyCreatorApplicationListFilters(countQ, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := dbutil.Val[int64](ctx, r.db, countQ)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	q := sq.Select(creatorApplicationListSelectColumns()...).
		From(TableCreatorApplications + " " + creatorApplicationListAlias).
		LeftJoin(creatorApplicationListTelegramJoin())
	if params.Sort == domain.CreatorApplicationSortCityName {
		q = q.LeftJoin(creatorApplicationListCityJoin())
	}
	q, err = applyCreatorApplicationListFilters(q, params)
	if err != nil {
		return nil, 0, err
	}
	q = applyCreatorApplicationListOrder(q, params.Sort, params.Order)

	offset := (params.Page - 1) * params.PerPage
	q = q.Limit(uint64(params.PerPage)).Offset(uint64(offset))

	rows, err := dbutil.Many[CreatorApplicationListRow](ctx, r.db, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// creatorApplicationListSelectColumns builds the explicit projection list for
// the page query. Every column from the table comes prefixed with the alias
// so the LEFT JOINs cannot accidentally introduce ambiguity, and the derived
// boolean is exposed under the column name pgx looks for via `db:"telegram_linked"`.
func creatorApplicationListSelectColumns() []string {
	caCol := func(c string) string {
		return creatorApplicationListAlias + "." + c + " AS " + c
	}
	return []string{
		caCol(CreatorApplicationColumnID),
		caCol(CreatorApplicationColumnLastName),
		caCol(CreatorApplicationColumnFirstName),
		caCol(CreatorApplicationColumnMiddleName),
		caCol(CreatorApplicationColumnBirthDate),
		caCol(CreatorApplicationColumnCityCode),
		caCol(CreatorApplicationColumnStatus),
		caCol(CreatorApplicationColumnCreatedAt),
		caCol(CreatorApplicationColumnUpdatedAt),
		"(" + creatorApplicationListTelegramAlias + "." +
			CreatorApplicationTelegramLinkColumnApplicationID + " IS NOT NULL) AS " +
			creatorApplicationListTelegramLinkedAs,
	}
}

func creatorApplicationListTelegramJoin() string {
	return TableCreatorApplicationTelegramLinks + " " + creatorApplicationListTelegramAlias +
		" ON " + creatorApplicationListTelegramAlias + "." +
		CreatorApplicationTelegramLinkColumnApplicationID + " = " +
		creatorApplicationListAlias + "." + CreatorApplicationColumnID
}

func creatorApplicationListCityJoin() string {
	return TableCities + " " + creatorApplicationListCityAlias +
		" ON " + creatorApplicationListCityAlias + "." + DictionaryColumnCode +
		" = " + creatorApplicationListAlias + "." + CreatorApplicationColumnCityCode
}

// applyCreatorApplicationListFilters appends every active filter condition.
// Multi-value arrays produce IN-clauses (any-of), the categories filter goes
// through an EXISTS subquery on the M:N table, and the search clause spans
// the four PII columns plus an EXISTS on socials.handle. Search is escaped
// for ILIKE wildcards (`%`, `_`, `\`) so an admin searching for "100%" or
// "_test" gets a literal substring match instead of Postgres' wildcard
// semantics. Returning an error mirrors the squirrel ToSql() contract — the
// categories sub-builder can theoretically fail on misuse, and we never
// want to silently feed `EXISTS ()` to Postgres.
func applyCreatorApplicationListFilters(q sq.SelectBuilder, p CreatorApplicationListParams) (sq.SelectBuilder, error) {
	caStatus := creatorApplicationListAlias + "." + CreatorApplicationColumnStatus
	caCity := creatorApplicationListAlias + "." + CreatorApplicationColumnCityCode
	caCreatedAt := creatorApplicationListAlias + "." + CreatorApplicationColumnCreatedAt
	caBirthDate := creatorApplicationListAlias + "." + CreatorApplicationColumnBirthDate
	caID := creatorApplicationListAlias + "." + CreatorApplicationColumnID
	tglAppID := creatorApplicationListTelegramAlias + "." + CreatorApplicationTelegramLinkColumnApplicationID

	if len(p.Statuses) > 0 {
		q = q.Where(sq.Eq{caStatus: p.Statuses})
	}
	if len(p.Cities) > 0 {
		q = q.Where(sq.Eq{caCity: p.Cities})
	}
	if len(p.Categories) > 0 {
		cacAppID := creatorApplicationListCategoryAlias + "." + CreatorApplicationCategoryColumnApplicationID
		cacCode := creatorApplicationListCategoryAlias + "." + CreatorApplicationCategoryColumnCategoryCode
		sub := sq.Select("1").
			From(TableCreatorApplicationCategories + " " + creatorApplicationListCategoryAlias).
			Where(cacAppID + " = " + caID).
			Where(sq.Eq{cacCode: p.Categories})
		subSQL, subArgs, err := sub.ToSql()
		if err != nil {
			return q, fmt.Errorf("build categories EXISTS subquery: %w", err)
		}
		q = q.Where(sq.Expr("EXISTS ("+subSQL+")", subArgs...))
	}
	if p.DateFrom != nil {
		q = q.Where(sq.GtOrEq{caCreatedAt: *p.DateFrom})
	}
	if p.DateTo != nil {
		q = q.Where(sq.LtOrEq{caCreatedAt: *p.DateTo})
	}
	if p.AgeFrom != nil {
		q = q.Where(sq.Expr(caBirthDate+" <= NOW()::date - make_interval(years => ?)", *p.AgeFrom))
	}
	if p.AgeTo != nil {
		q = q.Where(sq.Expr(caBirthDate+" > NOW()::date - make_interval(years => ?)", *p.AgeTo+1))
	}
	if p.TelegramLinked != nil {
		if *p.TelegramLinked {
			q = q.Where(tglAppID + " IS NOT NULL")
		} else {
			q = q.Where(tglAppID + " IS NULL")
		}
	}
	if p.Search != "" {
		caLastName := creatorApplicationListAlias + "." + CreatorApplicationColumnLastName
		caFirstName := creatorApplicationListAlias + "." + CreatorApplicationColumnFirstName
		caMiddleName := creatorApplicationListAlias + "." + CreatorApplicationColumnMiddleName
		caIIN := creatorApplicationListAlias + "." + CreatorApplicationColumnIIN
		casAppID := creatorApplicationListSocialAlias + "." + CreatorApplicationSocialColumnApplicationID
		casHandle := creatorApplicationListSocialAlias + "." + CreatorApplicationSocialColumnHandle
		pattern := "%" + escapeLikeWildcards(p.Search) + "%"
		q = q.Where(sq.Expr(
			"("+caLastName+` ILIKE ? ESCAPE '\' OR `+
				caFirstName+` ILIKE ? ESCAPE '\' OR `+
				caMiddleName+` ILIKE ? ESCAPE '\' OR `+
				caIIN+` ILIKE ? ESCAPE '\' OR `+
				"EXISTS (SELECT 1 FROM "+TableCreatorApplicationSocials+" "+creatorApplicationListSocialAlias+
				" WHERE "+casAppID+" = "+caID+
				` AND `+casHandle+` ILIKE ? ESCAPE '\'))`,
			pattern, pattern, pattern, pattern, pattern))
	}
	return q, nil
}

// escapeLikeWildcards neutralises the three special characters in Postgres
// LIKE/ILIKE patterns (`%`, `_`, `\`) so user-supplied search treats them as
// literals. Without this, searching for "100%" returns every row containing
// "100" (since `%` is the LIKE-any-string wildcard); searching for "_user"
// matches every row with any single character followed by "user". The
// generated SQL pairs each ILIKE with `ESCAPE '\'` so Postgres uses the
// backslash as the escape character (instead of relying on the standard
// default which differs across configurations).
func escapeLikeWildcards(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// applyCreatorApplicationListOrder picks the SQL ORDER BY clause for the
// validated sort field. Every branch tail-orders by id ASC so applications
// with equal sort keys stay deterministically ordered across pages and
// across direction flips — fixing the tie-breaker direction independently
// of the main sort means a page boundary captured at sort=DESC stays in the
// same place when the same query is later run with sort=ASC.
func applyCreatorApplicationListOrder(q sq.SelectBuilder, sort, order string) sq.SelectBuilder {
	dir := "ASC"
	if order == domain.SortOrderDesc {
		dir = "DESC"
	}
	tieBreaker := creatorApplicationListAlias + "." + CreatorApplicationColumnID + " ASC"
	switch sort {
	case domain.CreatorApplicationSortCreatedAt:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnCreatedAt
		return q.OrderBy(col+" "+dir, tieBreaker)
	case domain.CreatorApplicationSortUpdatedAt:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnUpdatedAt
		return q.OrderBy(col+" "+dir, tieBreaker)
	case domain.CreatorApplicationSortBirthDate:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnBirthDate
		return q.OrderBy(col+" "+dir, tieBreaker)
	case domain.CreatorApplicationSortFullName:
		last := creatorApplicationListAlias + "." + CreatorApplicationColumnLastName
		first := creatorApplicationListAlias + "." + CreatorApplicationColumnFirstName
		middle := creatorApplicationListAlias + "." + CreatorApplicationColumnMiddleName
		return q.OrderBy(last+" "+dir, first+" "+dir, middle+" "+dir, tieBreaker)
	case domain.CreatorApplicationSortCityName:
		name := creatorApplicationListCityAlias + "." + DictionaryColumnName
		return q.OrderBy(name+" "+dir, tieBreaker)
	default:
		// Service+handler reject unknown sort upstream; this branch is a
		// defensive fallback so a future drift never produces an unsorted
		// page. Logging here would require plumbing a logger into the repo —
		// out of scope; the validation layer is the right place to catch it.
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnCreatedAt
		return q.OrderBy(col+" DESC", tieBreaker)
	}
}
