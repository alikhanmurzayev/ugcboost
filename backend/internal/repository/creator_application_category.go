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
	TableCreatorApplicationCategories                 = "creator_application_categories"
	CreatorApplicationCategoryColumnID                = "id"
	CreatorApplicationCategoryColumnApplicationID     = "application_id"
	CreatorApplicationCategoryColumnCategoryID        = "category_id"
	CreatorApplicationCategoryColumnCreatedAt         = "created_at"
)

// CreatorApplicationCategoryRow maps to the creator_application_categories join table.
type CreatorApplicationCategoryRow struct {
	ID            string    `db:"id"`
	ApplicationID string    `db:"application_id" insert:"application_id"`
	CategoryID    string    `db:"category_id"    insert:"category_id"`
	CreatedAt     time.Time `db:"created_at"`
}

var (
	creatorApplicationCategoryInsertMapper  = stom.MustNewStom(CreatorApplicationCategoryRow{}).SetTag(string(tagInsert))
	creatorApplicationCategoryInsertColumns = sortColumns(creatorApplicationCategoryInsertMapper.TagValues())
)

// CreatorApplicationCategoryDetailRow carries the join-projected category
// fields used by the read aggregate. Sort order is preserved by the SQL ORDER
// BY clause; the row itself only describes the category, not the link row.
type CreatorApplicationCategoryDetailRow struct {
	Code      string `db:"code"`
	Name      string `db:"name"`
	SortOrder int    `db:"sort_order"`
}

// CreatorApplicationCategoryRepo batches the M:N link rows between an
// application and its selected categories.
type CreatorApplicationCategoryRepo interface {
	InsertMany(ctx context.Context, rows []CreatorApplicationCategoryRow) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationCategoryDetailRow, error)
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

// ListByApplicationID returns the categories linked to an application joined
// against the categories dictionary, sorted by (sort_order, code) so the
// response is deterministic. We deliberately do NOT filter by c.active —
// historical applications must still surface a category that has since been
// deactivated.
func (r *creatorApplicationCategoryRepository) ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationCategoryDetailRow, error) {
	q := sq.Select(
		"c."+DictionaryColumnCode,
		"c."+DictionaryColumnName,
		"c."+DictionaryColumnSortOrder,
	).
		From(TableCreatorApplicationCategories+" cac").
		Join(TableCategories+" c ON c."+DictionaryColumnID+" = cac."+CreatorApplicationCategoryColumnCategoryID).
		Where(sq.Eq{"cac." + CreatorApplicationCategoryColumnApplicationID: applicationID}).
		OrderBy("c."+DictionaryColumnSortOrder+" ASC", "c."+DictionaryColumnCode+" ASC")
	return dbutil.Many[CreatorApplicationCategoryDetailRow](ctx, r.db, q)
}
