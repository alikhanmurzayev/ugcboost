package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator application categories join table and column names.
const (
	TableCreatorApplicationCategories             = "creator_application_categories"
	CreatorApplicationCategoryColumnID            = "id"
	CreatorApplicationCategoryColumnApplicationID = "application_id"
	CreatorApplicationCategoryColumnCategoryCode  = "category_code"
	CreatorApplicationCategoryColumnCreatedAt     = "created_at"
)

// CreatorApplicationCategoryRow maps to the creator_application_categories join table.
type CreatorApplicationCategoryRow struct {
	ID            string    `db:"id"`
	ApplicationID string    `db:"application_id" insert:"application_id"`
	CategoryCode  string    `db:"category_code"  insert:"category_code"`
	CreatedAt     time.Time `db:"created_at"`
}

var (
	creatorApplicationCategoryInsertMapper  = stom.MustNewStom(CreatorApplicationCategoryRow{}).SetTag(string(tagInsert))
	creatorApplicationCategoryInsertColumns = sortColumns(creatorApplicationCategoryInsertMapper.TagValues())
)

// CreatorApplicationCategoryRepo batches the M:N link rows between an
// application and its selected categories.
type CreatorApplicationCategoryRepo interface {
	InsertMany(ctx context.Context, rows []CreatorApplicationCategoryRow) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]string, error)
	ListByApplicationIDs(ctx context.Context, applicationIDs []string) (map[string][]string, error)
}

type creatorApplicationCategoryRepository struct {
	db dbutil.DB
}

// InsertMany writes every link row in a single INSERT. An empty input is a
// no-op — callers decide whether zero categories is legal upstream.
func (r *creatorApplicationCategoryRepository) InsertMany(ctx context.Context, rows []CreatorApplicationCategoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	qb := sq.Insert(TableCreatorApplicationCategories).Columns(creatorApplicationCategoryInsertColumns...)
	for _, row := range rows {
		qb = insertEntities(qb, creatorApplicationCategoryInsertMapper, creatorApplicationCategoryInsertColumns, row)
	}
	_, err := dbutil.Exec(ctx, r.db, qb)
	return err
}

// ListByApplicationID returns the category codes linked to an application.
// No JOIN is needed: category_code is stored directly on the link row, so
// historical applications surface their codes even if the category has been
// deactivated. The handler resolves names against the active dictionary and
// re-sorts in-memory, so callers do not depend on this order for presentation.
func (r *creatorApplicationCategoryRepository) ListByApplicationID(ctx context.Context, applicationID string) ([]string, error) {
	q := sq.Select(CreatorApplicationCategoryColumnCategoryCode).
		From(TableCreatorApplicationCategories).
		Where(sq.Eq{CreatorApplicationCategoryColumnApplicationID: applicationID}).
		OrderBy(CreatorApplicationCategoryColumnCategoryCode + " ASC")
	return dbutil.Vals[string](ctx, r.db, q)
}

// creatorApplicationCategoryHydrationRow is a private projection used only by
// ListByApplicationIDs. It is kept off the public surface (the rest of the
// codebase reads single-application categories via ListByApplicationID) so
// the row shape can evolve with the batch use-case without breaking callers.
type creatorApplicationCategoryHydrationRow struct {
	ApplicationID string `db:"application_id"`
	CategoryCode  string `db:"category_code"`
}

// ListByApplicationIDs hydrates the categories of every supplied application
// id in a single query. It returns a map keyed by applicationID, with each
// slice already sorted by category code so the handler hydration step can
// merge with the dictionary lookup deterministically. An empty input set is a
// no-op and returns an empty map without hitting the database.
func (r *creatorApplicationCategoryRepository) ListByApplicationIDs(ctx context.Context, applicationIDs []string) (map[string][]string, error) {
	if len(applicationIDs) == 0 {
		return map[string][]string{}, nil
	}
	q := sq.Select(
		CreatorApplicationCategoryColumnApplicationID,
		CreatorApplicationCategoryColumnCategoryCode,
	).
		From(TableCreatorApplicationCategories).
		Where(sq.Eq{CreatorApplicationCategoryColumnApplicationID: applicationIDs}).
		OrderBy(
			CreatorApplicationCategoryColumnApplicationID+" ASC",
			CreatorApplicationCategoryColumnCategoryCode+" ASC",
		)
	rows, err := dbutil.Many[creatorApplicationCategoryHydrationRow](ctx, r.db, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(applicationIDs))
	for _, row := range rows {
		out[row.ApplicationID] = append(out[row.ApplicationID], row.CategoryCode)
	}
	return out, nil
}
