package repository

import (
	"context"
	"database/sql"
	"errors"
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
	DeleteForTests(ctx context.Context, id string) error
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
