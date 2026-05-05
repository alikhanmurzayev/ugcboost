package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator categories join table and column names.
const (
	TableCreatorCategories            = "creator_categories"
	CreatorCategoryColumnID           = "id"
	CreatorCategoryColumnCreatorID    = "creator_id"
	CreatorCategoryColumnCategoryCode = "category_code"
	CreatorCategoryColumnCreatedAt    = "created_at"
)

// CreatorCategoryRow maps to the creator_categories M:N link table.
type CreatorCategoryRow struct {
	ID           string    `db:"id"`
	CreatorID    string    `db:"creator_id"    insert:"creator_id"`
	CategoryCode string    `db:"category_code" insert:"category_code"`
	CreatedAt    time.Time `db:"created_at"`
}

var (
	creatorCategoryInsertMapper  = stom.MustNewStom(CreatorCategoryRow{}).SetTag(string(tagInsert))
	creatorCategoryInsertColumns = sortColumns(creatorCategoryInsertMapper.TagValues())
)

// CreatorCategoryRepo batches the M:N link rows between a creator and the
// categories they were approved under.
type CreatorCategoryRepo interface {
	InsertMany(ctx context.Context, rows []CreatorCategoryRow) error
	ListByCreatorID(ctx context.Context, creatorID string) ([]string, error)
}

type creatorCategoryRepository struct {
	db dbutil.DB
}

// InsertMany writes every link row in a single INSERT. Empty input is a
// no-op.
func (r *creatorCategoryRepository) InsertMany(ctx context.Context, rows []CreatorCategoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	qb := sq.Insert(TableCreatorCategories).Columns(creatorCategoryInsertColumns...)
	for _, row := range rows {
		qb = insertEntities(qb, creatorCategoryInsertMapper, creatorCategoryInsertColumns, row)
	}
	_, err := dbutil.Exec(ctx, r.db, qb)
	return err
}

// ListByCreatorID returns the category codes linked to the given creator
// in deterministic ascending order. The handler layer (18c) resolves the
// codes against the active dictionary at read time.
func (r *creatorCategoryRepository) ListByCreatorID(ctx context.Context, creatorID string) ([]string, error) {
	q := sq.Select(CreatorCategoryColumnCategoryCode).
		From(TableCreatorCategories).
		Where(sq.Eq{CreatorCategoryColumnCreatorID: creatorID}).
		OrderBy(CreatorCategoryColumnCategoryCode + " ASC")
	return dbutil.Vals[string](ctx, r.db, q)
}
