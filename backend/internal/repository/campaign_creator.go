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

// Campaign-creator constraint names. Matched against pgErr.ConstraintName so
// 23505 / 23503 races are translated into precise domain errors instead of
// leaking raw Postgres errors to callers.
const (
	CampaignCreatorsCampaignCreatorUnique = "campaign_creators_campaign_creator_unique"
	CampaignCreatorsCampaignIDFK          = "campaign_creators_campaign_id_fk"
	CampaignCreatorsCreatorIDFK           = "campaign_creators_creator_id_fk"
)

// campaign_creators table and column names.
const (
	TableCampaignCreators              = "campaign_creators"
	CampaignCreatorColumnID            = "id"
	CampaignCreatorColumnCampaignID    = "campaign_id"
	CampaignCreatorColumnCreatorID     = "creator_id"
	CampaignCreatorColumnStatus        = "status"
	CampaignCreatorColumnInvitedAt     = "invited_at"
	CampaignCreatorColumnInvitedCount  = "invited_count"
	CampaignCreatorColumnRemindedAt    = "reminded_at"
	CampaignCreatorColumnRemindedCount = "reminded_count"
	CampaignCreatorColumnDecidedAt     = "decided_at"
	CampaignCreatorColumnCreatedAt     = "created_at"
	CampaignCreatorColumnUpdatedAt     = "updated_at"
)

// CampaignCreatorRow maps to the campaign_creators table. Insert tags cover
// only the three fields the service supplies — id / counters / timestamps
// are DB-defaulted, nullable timestamps stay NULL until chunks 12+ flip them.
type CampaignCreatorRow struct {
	ID            string     `db:"id"`
	CampaignID    string     `db:"campaign_id"     insert:"campaign_id"`
	CreatorID     string     `db:"creator_id"      insert:"creator_id"`
	Status        string     `db:"status"          insert:"status"`
	InvitedAt     *time.Time `db:"invited_at"`
	InvitedCount  int        `db:"invited_count"`
	RemindedAt    *time.Time `db:"reminded_at"`
	RemindedCount int        `db:"reminded_count"`
	DecidedAt     *time.Time `db:"decided_at"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
}

var (
	campaignCreatorSelectColumns = sortColumns(stom.MustNewStom(CampaignCreatorRow{}).SetTag(string(tagSelect)).TagValues())
	campaignCreatorInsertMapper  = stom.MustNewStom(CampaignCreatorRow{}).SetTag(string(tagInsert))
)

// CampaignCreatorRepo lists every public method of the campaign_creators repo.
type CampaignCreatorRepo interface {
	Add(ctx context.Context, campaignID, creatorID, status string) (*CampaignCreatorRow, error)
	GetByCampaignAndCreator(ctx context.Context, campaignID, creatorID string) (*CampaignCreatorRow, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]*CampaignCreatorRow, error)
	ListByCampaignAndCreators(ctx context.Context, campaignID string, creatorIDs []string) ([]*CampaignCreatorRow, error)
	ApplyInvite(ctx context.Context, id string) (*CampaignCreatorRow, error)
	ApplyRemind(ctx context.Context, id string) (*CampaignCreatorRow, error)
	ExistsInvitedInCampaign(ctx context.Context, campaignID string) (bool, error)
	DeleteByID(ctx context.Context, id string) error
}

type campaignCreatorRepository struct {
	db dbutil.DB
}

// Add inserts a single campaign_creators row and returns the persisted row
// with DB-generated fields populated. Three race translations:
//
//   - 23505 + campaign_creators_campaign_creator_unique → ErrCreatorAlreadyInCampaign
//     (the (campaign, creator) pair was already attached).
//   - 23503 + campaign_creators_creator_id_fk → ErrCampaignCreatorCreatorNotFound
//     (one of the creatorIds does not point to a real creator — strict-422).
//   - 23503 + campaign_creators_campaign_id_fk → ErrCampaignNotFound. Soft-
//     delete (`is_deleted=true`) cannot trigger this branch — it is a flag,
//     not a DELETE. The branch only fires on hard-delete (test cleanup or
//     direct psql), so it is defensive against parallel cleanup tests.
//
// All other pgconn errors are propagated raw — the service wraps them with
// fmt.Errorf as usual.
func (r *campaignCreatorRepository) Add(ctx context.Context, campaignID, creatorID, status string) (*CampaignCreatorRow, error) {
	q := sq.Insert(TableCampaignCreators).
		SetMap(toMap(CampaignCreatorRow{
			CampaignID: campaignID,
			CreatorID:  creatorID,
			Status:     status,
		}, campaignCreatorInsertMapper)).
		Suffix(returningClause(campaignCreatorSelectColumns))
	row, err := dbutil.One[CampaignCreatorRow](ctx, r.db, q)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23505" && pgErr.ConstraintName == CampaignCreatorsCampaignCreatorUnique:
				return nil, domain.ErrCreatorAlreadyInCampaign
			case pgErr.Code == "23503" && pgErr.ConstraintName == CampaignCreatorsCreatorIDFK:
				return nil, domain.ErrCampaignCreatorCreatorNotFound
			case pgErr.Code == "23503" && pgErr.ConstraintName == CampaignCreatorsCampaignIDFK:
				return nil, domain.ErrCampaignNotFound
			}
		}
		return nil, err
	}
	return row, nil
}

// GetByCampaignAndCreator returns the row for the (campaign, creator) pair.
// dbutil.One wraps sql.ErrNoRows; the service translates it to
// ErrCampaignCreatorNotFound at the boundary.
func (r *campaignCreatorRepository) GetByCampaignAndCreator(ctx context.Context, campaignID, creatorID string) (*CampaignCreatorRow, error) {
	q := sq.Select(campaignCreatorSelectColumns...).
		From(TableCampaignCreators).
		Where(sq.Eq{
			CampaignCreatorColumnCampaignID: campaignID,
			CampaignCreatorColumnCreatorID:  creatorID,
		})
	return dbutil.One[CampaignCreatorRow](ctx, r.db, q)
}

// ListByCampaign returns every row for the campaign ordered by created_at
// ASC, id ASC so the admin UI sees a stable order across requests. There is
// no pagination — the roster fits one screen by design (chunk 10 spec).
func (r *campaignCreatorRepository) ListByCampaign(ctx context.Context, campaignID string) ([]*CampaignCreatorRow, error) {
	q := sq.Select(campaignCreatorSelectColumns...).
		From(TableCampaignCreators).
		Where(sq.Eq{CampaignCreatorColumnCampaignID: campaignID}).
		OrderBy(CampaignCreatorColumnCreatedAt+" ASC", CampaignCreatorColumnID+" ASC")
	return dbutil.Many[CampaignCreatorRow](ctx, r.db, q)
}

// DeleteByID hard-deletes a row by primary key. Returns sql.ErrNoRows when
// the row does not exist so the service can map it to the right domain
// error after pre-fetching the row for the audit snapshot.
func (r *campaignCreatorRepository) DeleteByID(ctx context.Context, id string) error {
	q := sq.Delete(TableCampaignCreators).Where(sq.Eq{CampaignCreatorColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListByCampaignAndCreators returns every campaign_creators row that matches
// the (campaign, creator IN ($creatorIds)) pair. Used by chunk 12 notify /
// remind-invitation as the read-only validation pre-pass: the service
// compares the result against the input batch to surface
// not_in_campaign / wrong_status details. Empty creatorIds returns an empty
// result without hitting the database.
func (r *campaignCreatorRepository) ListByCampaignAndCreators(ctx context.Context, campaignID string, creatorIDs []string) ([]*CampaignCreatorRow, error) {
	if len(creatorIDs) == 0 {
		return nil, nil
	}
	q := sq.Select(campaignCreatorSelectColumns...).
		From(TableCampaignCreators).
		Where(sq.Eq{
			CampaignCreatorColumnCampaignID: campaignID,
			CampaignCreatorColumnCreatorID:  creatorIDs,
		})
	return dbutil.Many[CampaignCreatorRow](ctx, r.db, q)
}

// ApplyInvite advances a campaign_creators row to status=invited and bumps
// the invitation counter. The CASE branches reset the decision-cycle fields
// (`reminded_count`, `reminded_at`, `decided_at`) only when the source
// status was `declined` — re-invite after refusal restarts the cycle as if
// the row had just been added. Coming from `planned`, those fields are
// already at their initial values, so the CASE is a no-op there. Returns
// the freshly updated row for audit. Caller (service) is responsible for
// the validation pre-pass: this method does not check the source status,
// it just trusts the caller-provided id.
//
// Postgres semantics note: every SET expression sees the OLD row state,
// so the CASE branches read `status` before the same statement flips it
// to `invited`. Do not refactor the SET ordering for "clarity" without
// preserving this property — moving `status` last would not change
// behaviour, but moving CASE columns into a multi-row CTE would.
func (r *campaignCreatorRepository) ApplyInvite(ctx context.Context, id string) (*CampaignCreatorRow, error) {
	resetIfDeclined := func(col string, fallback any) sq.Sqlizer {
		return sq.Expr(
			"CASE WHEN "+CampaignCreatorColumnStatus+" = ? THEN ? ELSE "+col+" END",
			domain.CampaignCreatorStatusDeclined, fallback,
		)
	}
	q := sq.Update(TableCampaignCreators).
		Set(CampaignCreatorColumnStatus, domain.CampaignCreatorStatusInvited).
		Set(CampaignCreatorColumnInvitedCount, sq.Expr(CampaignCreatorColumnInvitedCount+" + 1")).
		Set(CampaignCreatorColumnInvitedAt, sq.Expr("now()")).
		Set(CampaignCreatorColumnRemindedCount, resetIfDeclined(CampaignCreatorColumnRemindedCount, 0)).
		Set(CampaignCreatorColumnRemindedAt, resetIfDeclined(CampaignCreatorColumnRemindedAt, nil)).
		Set(CampaignCreatorColumnDecidedAt, resetIfDeclined(CampaignCreatorColumnDecidedAt, nil)).
		Set(CampaignCreatorColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{CampaignCreatorColumnID: id}).
		Suffix(returningClause(campaignCreatorSelectColumns))
	return dbutil.One[CampaignCreatorRow](ctx, r.db, q)
}

// ApplyRemind bumps reminded_count and reminded_at on a campaign_creators
// row without changing the status. Caller (service) verifies the row is in
// status=invited before calling — this method does not re-check.
func (r *campaignCreatorRepository) ApplyRemind(ctx context.Context, id string) (*CampaignCreatorRow, error) {
	q := sq.Update(TableCampaignCreators).
		Set(CampaignCreatorColumnRemindedCount, sq.Expr(CampaignCreatorColumnRemindedCount+" + 1")).
		Set(CampaignCreatorColumnRemindedAt, sq.Expr("now()")).
		Set(CampaignCreatorColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{CampaignCreatorColumnID: id}).
		Suffix(returningClause(campaignCreatorSelectColumns))
	return dbutil.One[CampaignCreatorRow](ctx, r.db, q)
}

// ExistsInvitedInCampaign returns true if any campaign_creators row in this
// campaign has invited_count > 0 — i.e. an invitation has been delivered at
// least once. Used by UpdateCampaign to lock tma_url against changes that
// would silently break already-delivered web_app links.
func (r *campaignCreatorRepository) ExistsInvitedInCampaign(ctx context.Context, campaignID string) (bool, error) {
	sub := sq.Select("1").
		From(TableCampaignCreators).
		Where(sq.Eq{CampaignCreatorColumnCampaignID: campaignID}).
		Where(sq.Gt{CampaignCreatorColumnInvitedCount: 0})
	q := sq.Select().Column(sq.Expr("EXISTS (?)", sub))
	return dbutil.Val[bool](ctx, r.db, q)
}
