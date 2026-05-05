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
	ListByCreatorIDs(ctx context.Context, creatorIDs []string) (map[string][]string, error)
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

// creatorCategoryHydrationRow is a private projection used only by
// ListByCreatorIDs. It is kept off the public surface so the row shape can
// evolve with the batch use-case without breaking callers.
type creatorCategoryHydrationRow struct {
	CreatorID    string `db:"creator_id"`
	CategoryCode string `db:"category_code"`
}

// ListByCreatorIDs hydrates the categories of every supplied creator id in a
// single query. It returns a map keyed by creatorID, with each slice already
// sorted by category code so the handler hydration step can merge with the
// dictionary lookup deterministically. An empty input set is a no-op and
// returns an empty map without hitting the database.
func (r *creatorCategoryRepository) ListByCreatorIDs(ctx context.Context, creatorIDs []string) (map[string][]string, error) {
	if len(creatorIDs) == 0 {
		return map[string][]string{}, nil
	}
	q := sq.Select(
		CreatorCategoryColumnCreatorID,
		CreatorCategoryColumnCategoryCode,
	).
		From(TableCreatorCategories).
		Where(sq.Eq{CreatorCategoryColumnCreatorID: creatorIDs}).
		OrderBy(
			CreatorCategoryColumnCreatorID+" ASC",
			CreatorCategoryColumnCategoryCode+" ASC",
		)
	rows, err := dbutil.Many[creatorCategoryHydrationRow](ctx, r.db, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(creatorIDs))
	for _, row := range rows {
		out[row.CreatorID] = append(out[row.CreatorID], row.CategoryCode)
	}
	return out, nil
}
