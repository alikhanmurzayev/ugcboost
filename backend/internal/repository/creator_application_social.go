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
	creatorApplicationSocialInsertMapper  = stom.MustNewStom(CreatorApplicationSocialRow{}).SetTag(string(tagInsert))
	creatorApplicationSocialInsertColumns = sortColumns(creatorApplicationSocialInsertMapper.TagValues())
)

// CreatorApplicationSocialRepo batches the social-account rows attached to an
// application. Multiple handles on the same platform are legal per domain rules.
type CreatorApplicationSocialRepo interface {
	InsertMany(ctx context.Context, rows []CreatorApplicationSocialRow) error
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
