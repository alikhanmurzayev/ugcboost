package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Dictionary tables. Each public dictionary maps to its own physical table —
// the service layer (service.dictionaryTables) holds the type→table mapping
// using these constants so no string literal escapes the repository package.
const (
	TableCategories = "categories"
	TableCities     = "cities"
)

// Common column names shared by every dictionary table. The schema is fixed
// for now: every dictionary row has id/code/name/active/sort_order/created_at.
// Per-dictionary metadata (e.g. region for cities) belongs on its own
// dedicated repo, not here.
const (
	DictionaryColumnID        = "id"
	DictionaryColumnCode      = "code"
	DictionaryColumnName      = "name"
	DictionaryColumnActive    = "active"
	DictionaryColumnSortOrder = "sort_order"
	DictionaryColumnCreatedAt = "created_at"
)

// DictionaryEntryRow maps to one row of any of the dictionary tables. No
// insert tags — dictionaries are seeded in migrations; runtime writes are
// not planned for MVP.
type DictionaryEntryRow struct {
	ID        string    `db:"id"`
	Code      string    `db:"code"`
	Name      string    `db:"name"`
	Active    bool      `db:"active"`
	SortOrder int       `db:"sort_order"`
	CreatedAt time.Time `db:"created_at"`
}

var dictionarySelectColumns = sortColumns(stom.MustNewStom(DictionaryEntryRow{}).SetTag(string(tagSelect)).TagValues())

// DictionaryRepo lists all public methods of the dictionary repository.
type DictionaryRepo interface {
	ListActive(ctx context.Context, table string) ([]*DictionaryEntryRow, error)
	GetActiveByCodes(ctx context.Context, table string, codes []string) ([]*DictionaryEntryRow, error)
}

type dictionaryRepository struct {
	db dbutil.DB
}

// ListActive returns every active row of the given dictionary table, ordered
// by (sort_order, code) so the UI gets a stable, deterministic listing.
func (r *dictionaryRepository) ListActive(ctx context.Context, table string) ([]*DictionaryEntryRow, error) {
	q := sq.Select(dictionarySelectColumns...).
		From(table).
		Where(sq.Eq{DictionaryColumnActive: true}).
		OrderBy(DictionaryColumnSortOrder, DictionaryColumnCode)
	return dbutil.Many[DictionaryEntryRow](ctx, r.db, q)
}

// GetActiveByCodes returns every active row whose code is in the given set.
// An empty input yields an empty result without hitting the database.
// Missing codes are simply absent from the result — callers compare counts
// and surface a typed error when something is unknown.
func (r *dictionaryRepository) GetActiveByCodes(ctx context.Context, table string, codes []string) ([]*DictionaryEntryRow, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	q := sq.Select(dictionarySelectColumns...).
		From(table).
		Where(sq.Eq{DictionaryColumnCode: codes}).
		Where(sq.Eq{DictionaryColumnActive: true})
	return dbutil.Many[DictionaryEntryRow](ctx, r.db, q)
}
