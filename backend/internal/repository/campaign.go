package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Campaign unique constraint names — matched against pgErr.ConstraintName so
// the 23505-switch translates concurrent races into precise domain sentinels
// instead of leaking the raw Postgres error.
const (
	CampaignsNameActiveUnique = "campaigns_name_active_unique"
)

// Campaigns table and column names.
const (
	TableCampaigns          = "campaigns"
	CampaignColumnID        = "id"
	CampaignColumnName      = "name"
	CampaignColumnTmaURL    = "tma_url"
	CampaignColumnIsDeleted = "is_deleted"
	CampaignColumnCreatedAt = "created_at"
	CampaignColumnUpdatedAt = "updated_at"
)

// CampaignRow maps to the campaigns table. Insert tags cover only the two
// fields the service supplies — id / is_deleted / *_at are DB-defaulted.
type CampaignRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"       insert:"name"`
	TmaURL    string    `db:"tma_url"    insert:"tma_url"`
	IsDeleted bool      `db:"is_deleted"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

var (
	campaignSelectColumns = sortColumns(stom.MustNewStom(CampaignRow{}).SetTag(string(tagSelect)).TagValues())
	campaignInsertMapper  = stom.MustNewStom(CampaignRow{}).SetTag(string(tagInsert))
)

// CampaignRepo lists every public method of the campaign repository.
type CampaignRepo interface {
	Create(ctx context.Context, name, tmaURL string) (*CampaignRow, error)
	GetByID(ctx context.Context, id string) (*CampaignRow, error)
	Update(ctx context.Context, id, name, tmaURL string) (*CampaignRow, error)
	DeleteForTests(ctx context.Context, id string) error
}

type campaignRepository struct {
	db dbutil.DB
}

// Create inserts a new campaign row and returns the persisted row with
// DB-generated fields populated. Concurrent inserts of the same name on the
// partial unique index `campaigns_name_active_unique` (WHERE is_deleted =
// false) trip a 23505 — translated into domain.ErrCampaignNameTaken so the
// service surfaces a 409 instead of leaking the raw Postgres error.
func (r *campaignRepository) Create(ctx context.Context, name, tmaURL string) (*CampaignRow, error) {
	q := sq.Insert(TableCampaigns).
		SetMap(toMap(CampaignRow{Name: name, TmaURL: tmaURL}, campaignInsertMapper)).
		Suffix(returningClause(campaignSelectColumns))
	row, err := dbutil.One[CampaignRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == CampaignsNameActiveUnique {
			return nil, domain.ErrCampaignNameTaken
		}
		return nil, err
	}
	return row, nil
}

// GetByID returns the campaign row by id. The WHERE clause intentionally has
// no is_deleted filter — admin reads include soft-deleted campaigns so the
// moderation UI can audit and restore deletions; the live/deleted split lives
// in the upcoming list endpoint. dbutil.One propagates sql.ErrNoRows wrapped;
// the service forwards it untouched so the handler maps it to 404 via
// errors.Is, mirroring the creatorRepository.GetByID contract.
func (r *campaignRepository) GetByID(ctx context.Context, id string) (*CampaignRow, error) {
	q := sq.Select(campaignSelectColumns...).
		From(TableCampaigns).
		Where(sq.Eq{CampaignColumnID: id})
	return dbutil.One[CampaignRow](ctx, r.db, q)
}

// Update applies a full-replace of the mutable subset (name, tma_url) plus
// updated_at = now() and returns the post-update row through RETURNING. The
// WHERE clause has no is_deleted filter — admin PATCH may target soft-deleted
// rows for typo fixes, mirroring GetByID. dbutil.One propagates sql.ErrNoRows
// (wrapped) when no row matches; the service maps it to ErrCampaignNotFound.
// Concurrent renames against the partial unique index
// campaigns_name_active_unique trip a 23505 — translated into
// domain.ErrCampaignNameTaken so the service surfaces a 409 instead of
// leaking the raw Postgres error.
func (r *campaignRepository) Update(ctx context.Context, id, name, tmaURL string) (*CampaignRow, error) {
	q := sq.Update(TableCampaigns).
		Set(CampaignColumnName, name).
		Set(CampaignColumnTmaURL, tmaURL).
		Set(CampaignColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{CampaignColumnID: id}).
		Suffix(returningClause(campaignSelectColumns))
	row, err := dbutil.One[CampaignRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == CampaignsNameActiveUnique {
			return nil, domain.ErrCampaignNameTaken
		}
		return nil, err
	}
	return row, nil
}

// DeleteForTests hard-deletes a campaign by id. Returns sql.ErrNoRows when
// the campaign does not exist, matching the cleanup-stack semantics where
// "already gone" is treated as success at the caller.
func (r *campaignRepository) DeleteForTests(ctx context.Context, id string) error {
	q := sq.Delete(TableCampaigns).Where(sq.Eq{CampaignColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
