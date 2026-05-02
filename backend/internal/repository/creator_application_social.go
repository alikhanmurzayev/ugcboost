package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator application socials table and column names.
const (
	TableCreatorApplicationSocials              = "creator_application_socials"
	CreatorApplicationSocialColumnID            = "id"
	CreatorApplicationSocialColumnApplicationID = "application_id"
	CreatorApplicationSocialColumnPlatform      = "platform"
	CreatorApplicationSocialColumnHandle        = "handle"
	CreatorApplicationSocialColumnCreatedAt     = "created_at"
)

// CreatorApplicationSocialRow maps to the creator_application_socials table.
type CreatorApplicationSocialRow struct {
	ID            string    `db:"id"`
	ApplicationID string    `db:"application_id" insert:"application_id"`
	Platform      string    `db:"platform"       insert:"platform"`
	Handle        string    `db:"handle"         insert:"handle"`
	CreatedAt     time.Time `db:"created_at"`
}

var (
	creatorApplicationSocialSelectColumns = sortColumns(stom.MustNewStom(CreatorApplicationSocialRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationSocialInsertMapper  = stom.MustNewStom(CreatorApplicationSocialRow{}).SetTag(string(tagInsert))
	creatorApplicationSocialInsertColumns = sortColumns(creatorApplicationSocialInsertMapper.TagValues())
)

// CreatorApplicationSocialRepo batches the social-account rows attached to an
// application. Multiple handles on the same platform are legal per domain rules.
type CreatorApplicationSocialRepo interface {
	InsertMany(ctx context.Context, rows []CreatorApplicationSocialRow) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationSocialRow, error)
	ListByApplicationIDs(ctx context.Context, applicationIDs []string) (map[string][]*CreatorApplicationSocialRow, error)
}

type creatorApplicationSocialRepository struct {
	db dbutil.DB
}

// InsertMany writes every social account in a single INSERT. Empty input is a
// no-op.
func (r *creatorApplicationSocialRepository) InsertMany(ctx context.Context, rows []CreatorApplicationSocialRow) error {
	if len(rows) == 0 {
		return nil
	}
	qb := sq.Insert(TableCreatorApplicationSocials).Columns(creatorApplicationSocialInsertColumns...)
	for _, row := range rows {
		qb = insertEntities(qb, creatorApplicationSocialInsertMapper, creatorApplicationSocialInsertColumns, row)
	}
	_, err := dbutil.Exec(ctx, r.db, qb)
	return err
}

// ListByApplicationID returns every social account row tied to the given
// application, sorted by (platform, handle) so the response is deterministic.
func (r *creatorApplicationSocialRepository) ListByApplicationID(ctx context.Context, applicationID string) ([]*CreatorApplicationSocialRow, error) {
	q := sq.Select(creatorApplicationSocialSelectColumns...).
		From(TableCreatorApplicationSocials).
		Where(sq.Eq{CreatorApplicationSocialColumnApplicationID: applicationID}).
		OrderBy(CreatorApplicationSocialColumnPlatform+" ASC", CreatorApplicationSocialColumnHandle+" ASC")
	return dbutil.Many[CreatorApplicationSocialRow](ctx, r.db, q)
}

// ListByApplicationIDs hydrates social accounts for every supplied application
// id in a single query. The returned map is keyed by application id, and each
// slice keeps the same (platform, handle) ordering as ListByApplicationID so
// the handler hydration is deterministic. An empty input set is a no-op.
func (r *creatorApplicationSocialRepository) ListByApplicationIDs(ctx context.Context, applicationIDs []string) (map[string][]*CreatorApplicationSocialRow, error) {
	if len(applicationIDs) == 0 {
		return map[string][]*CreatorApplicationSocialRow{}, nil
	}
	q := sq.Select(creatorApplicationSocialSelectColumns...).
		From(TableCreatorApplicationSocials).
		Where(sq.Eq{CreatorApplicationSocialColumnApplicationID: applicationIDs}).
		OrderBy(
			CreatorApplicationSocialColumnApplicationID+" ASC",
			CreatorApplicationSocialColumnPlatform+" ASC",
			CreatorApplicationSocialColumnHandle+" ASC",
		)
	rows, err := dbutil.Many[CreatorApplicationSocialRow](ctx, r.db, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]*CreatorApplicationSocialRow, len(applicationIDs))
	for _, row := range rows {
		out[row.ApplicationID] = append(out[row.ApplicationID], row)
	}
	return out, nil
}
