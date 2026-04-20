package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Categories table and column names.
const (
	TableCategories          = "categories"
	CategoryColumnID         = "id"
	CategoryColumnCode       = "code"
	CategoryColumnName       = "name"
	CategoryColumnActive     = "active"
	CategoryColumnCreatedAt  = "created_at"
)

// CategoryRow maps to the categories table. No insert tags — the catalogue is
// seeded in migrations; no runtime writes are planned for MVP.
type CategoryRow struct {
	ID        string    `db:"id"`
	Code      string    `db:"code"`
	Name      string    `db:"name"`
	Active    bool      `db:"active"`
	CreatedAt time.Time `db:"created_at"`
}

var categorySelectColumns = sortColumns(stom.MustNewStom(CategoryRow{}).SetTag(string(tagSelect)).TagValues())

// CategoryRepo lists all public methods of the category repository.
type CategoryRepo interface {
	GetActiveByCodes(ctx context.Context, codes []string) ([]*CategoryRow, error)
}

type categoryRepository struct {
	db dbutil.DB
}

// GetActiveByCodes returns all active category rows whose code is in the given
// set. An empty input yields an empty result without hitting the database.
// Missing codes are simply absent from the result — callers are expected to
// compare the returned count with the requested count and surface a typed
// error when something is unknown.
func (r *categoryRepository) GetActiveByCodes(ctx context.Context, codes []string) ([]*CategoryRow, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	q := sq.Select(categorySelectColumns...).
		From(TableCategories).
		Where(sq.Eq{CategoryColumnCode: codes}).
		Where(sq.Eq{CategoryColumnActive: true})
	return dbutil.Many[CategoryRow](ctx, r.db, q)
}
