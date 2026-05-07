package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	ListByIDs(ctx context.Context, ids []string) ([]*CampaignRow, error)
	Update(ctx context.Context, id, name, tmaURL string) (*CampaignRow, error)
	List(ctx context.Context, params CampaignListParams) ([]*CampaignRow, int64, error)
	DeleteForTests(ctx context.Context, id string) error
}

// CampaignListParams carries the validated search/filter/sort/pagination
// inputs the service hands to the repo. The repo trusts these values
// (sort/order whitelisted, page/perPage bounded) and builds SQL directly.
type CampaignListParams struct {
	Search    string
	IsDeleted *bool
	Sort      string
	Order     string
	Page      int
	PerPage   int
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

// ListByIDs returns every campaign row whose id is in the given set, with no
// is_deleted filter — the caller decides what to do with soft-deleted rows.
// Empty input yields an empty result without hitting the database. Missing
// ids are simply absent from the result; the caller compares counts to
// surface a typed error when something is unknown or soft-deleted.
func (r *campaignRepository) ListByIDs(ctx context.Context, ids []string) ([]*CampaignRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := sq.Select(campaignSelectColumns...).
		From(TableCampaigns).
		Where(sq.Eq{CampaignColumnID: ids})
	rows, err := dbutil.Many[CampaignRow](ctx, r.db, q)
	if err != nil {
		return nil, fmt.Errorf("campaign_repository.ListByIDs: %w", err)
	}
	return rows, nil
}

// Update writes name/tma_url + updated_at=now() and RETURNINGs the row.
// is_deleted is not filtered here — the gate lives in CampaignService.
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

// List returns one page of campaigns matching the filter set, plus the
// unpaginated total. Page-q and count-q share the same WHERE-chain via
// applyCampaignListFilters so total is always consistent with items.
//
// Defensive bounds check: handler validates the range, but a future
// re-caller could pass garbage; without this, `int → uint64` silently wraps
// a negative offset into a giant unsigned number.
func (r *campaignRepository) List(ctx context.Context, params CampaignListParams) ([]*CampaignRow, int64, error) {
	if params.Page < 1 || params.PerPage < 1 {
		return nil, 0, fmt.Errorf("campaign_repository.List: invalid pagination page=%d perPage=%d", params.Page, params.PerPage)
	}

	countQ := applyCampaignListFilters(sq.Select("COUNT(*)").From(TableCampaigns), params)
	total, err := dbutil.Val[int64](ctx, r.db, countQ)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	q := sq.Select(campaignSelectColumns...).From(TableCampaigns)
	q = applyCampaignListFilters(q, params)
	q, err = applyCampaignListOrder(q, params.Sort, params.Order)
	if err != nil {
		return nil, 0, err
	}
	offset := (params.Page - 1) * params.PerPage
	q = q.Limit(uint64(params.PerPage)).Offset(uint64(offset))

	rows, err := dbutil.Many[CampaignRow](ctx, r.db, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// applyCampaignListFilters appends the active filters. ILIKE wildcards in
// search (`%`, `_`, `\`) are escaped so an admin searching for "100%" gets
// a literal substring match instead of Postgres' wildcard semantics.
func applyCampaignListFilters(q sq.SelectBuilder, p CampaignListParams) sq.SelectBuilder {
	if p.IsDeleted != nil {
		q = q.Where(sq.Eq{CampaignColumnIsDeleted: *p.IsDeleted})
	}
	if p.Search != "" {
		pattern := "%" + escapeLikeWildcards(p.Search) + "%"
		q = q.Where(sq.Expr(CampaignColumnName+` ILIKE ? ESCAPE '\'`, pattern))
	}
	return q
}

// applyCampaignListOrder picks ORDER BY for the validated sort. Every branch
// tail-orders by id ASC so rows with equal sort keys stay stable across
// pages and direction flips. Unknown sort returns a wrapped error rather
// than silently falling back — handler+service reject upstream.
func applyCampaignListOrder(q sq.SelectBuilder, sort, order string) (sq.SelectBuilder, error) {
	dir := "ASC"
	if order == domain.SortOrderDesc {
		dir = "DESC"
	}
	tieBreaker := CampaignColumnID + " ASC"
	switch sort {
	case domain.CampaignSortCreatedAt:
		return q.OrderBy(CampaignColumnCreatedAt+" "+dir, tieBreaker), nil
	case domain.CampaignSortUpdatedAt:
		return q.OrderBy(CampaignColumnUpdatedAt+" "+dir, tieBreaker), nil
	case domain.CampaignSortName:
		return q.OrderBy(CampaignColumnName+" "+dir, tieBreaker), nil
	default:
		return q, fmt.Errorf("campaign_repository.applyCampaignListOrder: unsupported sort %q", sort)
	}
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
