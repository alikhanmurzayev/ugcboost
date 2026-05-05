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

const (
	CreatorApplicationsIINActiveIdx                    = "creator_applications_iin_active_idx"
	CreatorApplicationsVerificationCodeVerificationIdx = "creator_applications_verification_code_verification_idx"
)

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
	CreatorApplicationColumnVerificationCode  = "verification_code"
	CreatorApplicationColumnCreatedAt         = "created_at"
	CreatorApplicationColumnUpdatedAt         = "updated_at"
)

// CreatorApplicationRow maps to the creator_applications table. Status and
// VerificationCode are passed in by the service (no DB DEFAULT) and carry
// insert tags. Future moderation flows update those columns via dedicated
// endpoints, not via the standard INSERT path.
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
	VerificationCode  string    `db:"verification_code"   insert:"verification_code"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

var (
	creatorApplicationSelectColumns  = sortColumns(stom.MustNewStom(CreatorApplicationRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationInsertMapper   = stom.MustNewStom(CreatorApplicationRow{}).SetTag(string(tagInsert))
	creatorApplicationListRowColumns = sortColumns(stom.MustNewStom(CreatorApplicationListRow{}).SetTag(string(tagSelect)).TagValues())
	// telegramLinked is a derived projection ((tgl.application_id IS NOT NULL))
	// rather than a real column on creator_applications; aliasing it as ca.* would
	// blow up the page query, so it is split off and emitted as an explicit
	// expression below.
	creatorApplicationListTableColumns = filterOutColumn(creatorApplicationListRowColumns, creatorApplicationListTelegramLinkedAs)
	creatorApplicationListProjection   = append(
		aliasedColumns(creatorApplicationListAlias, creatorApplicationListTableColumns),
		"("+creatorApplicationListTelegramAlias+"."+CreatorApplicationTelegramLinkColumnApplicationID+
			" IS NOT NULL) AS "+creatorApplicationListTelegramLinkedAs,
	)
)

// filterOutColumn returns cols without the given column, preserving order. The
// list-projection above relies on this to keep telegram_linked out of the
// alias-prefixed table columns.
func filterOutColumn(cols []string, drop string) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		if c == drop {
			continue
		}
		out = append(out, c)
	}
	return out
}

// CreatorApplicationRepo lists all public methods of the creator application
// repository.
type CreatorApplicationRepo interface {
	HasActiveByIIN(ctx context.Context, iin string) (bool, error)
	Create(ctx context.Context, row CreatorApplicationRow) (*CreatorApplicationRow, error)
	GetByID(ctx context.Context, id string) (*CreatorApplicationRow, error)
	GetByIDForUpdate(ctx context.Context, id string) (*CreatorApplicationRow, error)
	GetByVerificationCodeAndStatus(ctx context.Context, code, status string) (*CreatorApplicationRow, error)
	UpdateStatus(ctx context.Context, id, status string) error
	List(ctx context.Context, params CreatorApplicationListParams) ([]*CreatorApplicationListRow, int64, error)
	Counts(ctx context.Context) (map[string]int64, error)
	DeleteForTests(ctx context.Context, id string) error
}

// CreatorApplicationCountRow is the projected row shape of the GROUP BY status
// query used by Counts. Status values are unique by construction — `GROUP BY
// status` collapses duplicates at the SQL level — so a future map insert never
// overwrites a previous bucket.
type CreatorApplicationCountRow struct {
	Status string `db:"status"`
	Count  int64  `db:"count"`
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
// DB-generated fields populated (id, created_at, updated_at).
//
// Two partial unique indexes can fire SQLSTATE 23505 here: the IIN-active
// index (concurrent submit lost the race after HasActiveByIIN) and the
// verification_code-verification index (random 6-digit code happened to clash
// with another verification-status row). Each maps to a distinct domain
// sentinel — the service answers 409 on the first and retries on the second.
func (r *creatorApplicationRepository) Create(ctx context.Context, row CreatorApplicationRow) (*CreatorApplicationRow, error) {
	q := sq.Insert(TableCreatorApplications).
		SetMap(toMap(row, creatorApplicationInsertMapper)).
		Suffix(returningClause(creatorApplicationSelectColumns))
	result, err := dbutil.One[CreatorApplicationRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case CreatorApplicationsIINActiveIdx:
				return nil, domain.ErrCreatorApplicationDuplicate
			case CreatorApplicationsVerificationCodeVerificationIdx:
				return nil, domain.ErrCreatorApplicationVerificationCodeConflict
			}
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

// GetByIDForUpdate fetches the application row taking a row-level lock for
// the duration of the surrounding transaction. Concurrent transactions that
// hit the same row block here until the holder commits or rolls back. The
// approve flow calls this so two parallel approves of the same application
// serialise on the row: the second transaction wakes up after the first
// commits, sees status='approved', and returns NotApprovable instead of
// racing on a creator INSERT and surfacing an unrelated unique-constraint
// name (Postgres reports indexes in oid order, not by relevance).
func (r *creatorApplicationRepository) GetByIDForUpdate(ctx context.Context, id string) (*CreatorApplicationRow, error) {
	q := sq.Select(creatorApplicationSelectColumns...).
		From(TableCreatorApplications).
		Where(sq.Eq{CreatorApplicationColumnID: id}).
		Suffix("FOR UPDATE")
	return dbutil.One[CreatorApplicationRow](ctx, r.db, q)
}

// GetByVerificationCodeAndStatus returns the single application row with the
// given verification_code currently sitting in `status`. The
// (verification_code, status='verification') partial unique index guarantees
// at most one row, so this never sees a multi-row result. sql.ErrNoRows is
// propagated wrapped — the SendPulse service interprets that as the
// not_found no-op branch.
func (r *creatorApplicationRepository) GetByVerificationCodeAndStatus(ctx context.Context, code, status string) (*CreatorApplicationRow, error) {
	q := sq.Select(creatorApplicationSelectColumns...).
		From(TableCreatorApplications).
		Where(sq.Eq{CreatorApplicationColumnVerificationCode: code}).
		Where(sq.Eq{CreatorApplicationColumnStatus: status})
	return dbutil.One[CreatorApplicationRow](ctx, r.db, q)
}

// UpdateStatus moves an application to a new status, refreshing updated_at
// to NOW(). The state-machine guard lives in the service layer (via
// domain.IsCreatorApplicationTransitionAllowed) — this method is a dumb
// SQL wrapper. Returns sql.ErrNoRows when no row matches the id.
func (r *creatorApplicationRepository) UpdateStatus(ctx context.Context, id, status string) error {
	q := sq.Update(TableCreatorApplications).
		Set(CreatorApplicationColumnStatus, status).
		Set(CreatorApplicationColumnUpdatedAt, sq.Expr("NOW()")).
		Where(sq.Eq{CreatorApplicationColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
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

	countQ := applyCreatorApplicationListFilters(
		sq.Select("COUNT(*)").
			From(TableCreatorApplications+" "+creatorApplicationListAlias).
			LeftJoin(creatorApplicationListTelegramJoin()),
		params,
	)
	total, err := dbutil.Val[int64](ctx, r.db, countQ)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	q := sq.Select(creatorApplicationListProjection...).
		From(TableCreatorApplications + " " + creatorApplicationListAlias).
		LeftJoin(creatorApplicationListTelegramJoin())
	if params.Sort == domain.CreatorApplicationSortCityName {
		q = q.LeftJoin(creatorApplicationListCityJoin())
	}
	q = applyCreatorApplicationListFilters(q, params)
	q, err = applyCreatorApplicationListOrder(q, params.Sort, params.Order)
	if err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	q = q.Limit(uint64(params.PerPage)).Offset(uint64(offset))

	rows, err := dbutil.Many[CreatorApplicationListRow](ctx, r.db, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Counts returns the number of creator applications grouped by status. The
// result is a sparse map — only statuses that currently have at least one row
// in the table are present. Empty table → empty map. The single SQL
// `SELECT status, COUNT(*) FROM creator_applications GROUP BY status` is
// cheap (status is part of the partial unique index, status enum is small)
// and fits the read-only, no-filter contract of the admin counts endpoint.
func (r *creatorApplicationRepository) Counts(ctx context.Context) (map[string]int64, error) {
	q := sq.Select(CreatorApplicationColumnStatus, "COUNT(*) AS count").
		From(TableCreatorApplications).
		GroupBy(CreatorApplicationColumnStatus)
	rows, err := dbutil.Many[CreatorApplicationCountRow](ctx, r.db, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.Count
	}
	return out, nil
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
// semantics. Subqueries are passed to squirrel as Sqlizer objects (no manual
// ToSql round-trip), which keeps argument numbering correct and avoids
// silently feeding `EXISTS ()` to Postgres on a misuse.
func applyCreatorApplicationListFilters(q sq.SelectBuilder, p CreatorApplicationListParams) sq.SelectBuilder {
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
			// Cross-column equality — squirrel.Eq parameterises the RHS, so a
			// raw Where with column-name constants is the only way to express
			// it. Both sides are package constants; user input never enters.
			Where(cacAppID + " = " + caID).
			Where(sq.Eq{cacCode: p.Categories})
		q = q.Where(sq.Expr("EXISTS (?)", sub))
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
		socialsSub := sq.Select("1").
			From(TableCreatorApplicationSocials + " " + creatorApplicationListSocialAlias).
			Where(casAppID + " = " + caID).
			Where(sq.Expr(casHandle+` ILIKE ? ESCAPE '\'`, pattern))
		q = q.Where(sq.Or{
			sq.Expr(caLastName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(caFirstName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(caMiddleName+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr(caIIN+` ILIKE ? ESCAPE '\'`, pattern),
			sq.Expr("EXISTS (?)", socialsSub),
		})
	}
	return q
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
//
// Unknown sort returns a wrapped error rather than a silent fallback: the
// service+handler reject unknown sort upstream, so reaching this branch means
// a code-level drift, and silently sorting by created_at would mask the bug.
func applyCreatorApplicationListOrder(q sq.SelectBuilder, sort, order string) (sq.SelectBuilder, error) {
	dir := "ASC"
	if order == domain.SortOrderDesc {
		dir = "DESC"
	}
	tieBreaker := creatorApplicationListAlias + "." + CreatorApplicationColumnID + " ASC"
	switch sort {
	case domain.CreatorApplicationSortCreatedAt:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnCreatedAt
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorApplicationSortUpdatedAt:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnUpdatedAt
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorApplicationSortBirthDate:
		col := creatorApplicationListAlias + "." + CreatorApplicationColumnBirthDate
		return q.OrderBy(col+" "+dir, tieBreaker), nil
	case domain.CreatorApplicationSortFullName:
		last := creatorApplicationListAlias + "." + CreatorApplicationColumnLastName
		first := creatorApplicationListAlias + "." + CreatorApplicationColumnFirstName
		middle := creatorApplicationListAlias + "." + CreatorApplicationColumnMiddleName
		return q.OrderBy(last+" "+dir, first+" "+dir, middle+" "+dir, tieBreaker), nil
	case domain.CreatorApplicationSortCityName:
		name := creatorApplicationListCityAlias + "." + DictionaryColumnName
		return q.OrderBy(name+" "+dir, tieBreaker), nil
	default:
		return q, fmt.Errorf("creator_application_repository.applyCreatorApplicationListOrder: unsupported sort %q", sort)
	}
}
