package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator socials table and column names.
const (
	TableCreatorSocials                 = "creator_socials"
	CreatorSocialColumnID               = "id"
	CreatorSocialColumnCreatorID        = "creator_id"
	CreatorSocialColumnPlatform         = "platform"
	CreatorSocialColumnHandle           = "handle"
	CreatorSocialColumnVerified         = "verified"
	CreatorSocialColumnMethod           = "method"
	CreatorSocialColumnVerifiedByUserID = "verified_by_user_id"
	CreatorSocialColumnVerifiedAt       = "verified_at"
	CreatorSocialColumnCreatedAt        = "created_at"
)

// CreatorSocialRow maps to the creator_socials table. Verified / Method /
// VerifiedByUserID / VerifiedAt are insert-tagged: chunk 18b copies the
// verification snapshot from the application's social row, so they enter
// the table as the values picked by the service rather than DB defaults.
type CreatorSocialRow struct {
	ID               string     `db:"id"`
	CreatorID        string     `db:"creator_id"          insert:"creator_id"`
	Platform         string     `db:"platform"            insert:"platform"`
	Handle           string     `db:"handle"              insert:"handle"`
	Verified         bool       `db:"verified"            insert:"verified"`
	Method           *string    `db:"method"              insert:"method"`
	VerifiedByUserID *string    `db:"verified_by_user_id" insert:"verified_by_user_id"`
	VerifiedAt       *time.Time `db:"verified_at"         insert:"verified_at"`
	CreatedAt        time.Time  `db:"created_at"`
}

var (
	creatorSocialSelectColumns = sortColumns(stom.MustNewStom(CreatorSocialRow{}).SetTag(string(tagSelect)).TagValues())
	creatorSocialInsertMapper  = stom.MustNewStom(CreatorSocialRow{}).SetTag(string(tagInsert))
	creatorSocialInsertColumns = sortColumns(creatorSocialInsertMapper.TagValues())
)

// CreatorSocialRepo batches the social-account snapshot rows attached to a
// creator. Multiple handles on the same platform are allowed by domain
// rules; uniqueness is on (creator_id, platform, handle).
type CreatorSocialRepo interface {
	InsertMany(ctx context.Context, rows []CreatorSocialRow) error
	ListByCreatorIDs(ctx context.Context, creatorIDs []string) (map[string][]*CreatorSocialRow, error)
}

type creatorSocialRepository struct {
	db dbutil.DB
}

// InsertMany writes every social row in a single INSERT. Empty input is a
// no-op and never hits the database.
func (r *creatorSocialRepository) InsertMany(ctx context.Context, rows []CreatorSocialRow) error {
	if len(rows) == 0 {
		return nil
	}
	qb := sq.Insert(TableCreatorSocials).Columns(creatorSocialInsertColumns...)
	for _, row := range rows {
		qb = insertEntities(qb, creatorSocialInsertMapper, creatorSocialInsertColumns, row)
	}
	_, err := dbutil.Exec(ctx, r.db, qb)
	return err
}

// ListByCreatorIDs hydrates social accounts for every supplied creator id in a
// single query. The returned map is keyed by creator id; each slice keeps the
// (platform, handle) ordering so callers (single-creator GET aggregate, list
// hydration) can rely on a deterministic projection. An empty input set is a
// no-op and returns an empty map without hitting the database.
func (r *creatorSocialRepository) ListByCreatorIDs(ctx context.Context, creatorIDs []string) (map[string][]*CreatorSocialRow, error) {
	if len(creatorIDs) == 0 {
		return map[string][]*CreatorSocialRow{}, nil
	}
	q := sq.Select(creatorSocialSelectColumns...).
		From(TableCreatorSocials).
		Where(sq.Eq{CreatorSocialColumnCreatorID: creatorIDs}).
		OrderBy(
			CreatorSocialColumnCreatorID+" ASC",
			CreatorSocialColumnPlatform+" ASC",
			CreatorSocialColumnHandle+" ASC",
		)
	rows, err := dbutil.Many[CreatorSocialRow](ctx, r.db, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]*CreatorSocialRow, len(creatorIDs))
	for _, row := range rows {
		out[row.CreatorID] = append(out[row.CreatorID], row)
	}
	return out, nil
}
