package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Creator application status transitions table and column names.
const (
	TableCreatorApplicationStatusTransitions = "creator_application_status_transitions"

	CreatorApplicationStatusTransitionColumnID            = "id"
	CreatorApplicationStatusTransitionColumnApplicationID = "application_id"
	CreatorApplicationStatusTransitionColumnFromStatus    = "from_status"
	CreatorApplicationStatusTransitionColumnToStatus      = "to_status"
	CreatorApplicationStatusTransitionColumnActorID       = "actor_id"
	CreatorApplicationStatusTransitionColumnReason        = "reason"
	CreatorApplicationStatusTransitionColumnCreatedAt     = "created_at"
)

// CreatorApplicationStatusTransitionRow maps to
// creator_application_status_transitions. id and created_at default to
// gen_random_uuid()/NOW() in the migration so they stay out of the insert
// tags. from_status, actor_id and reason are nullable to accommodate
// system-driven transitions (no actor) and forward-compatible reasons.
type CreatorApplicationStatusTransitionRow struct {
	ID            string    `db:"id"`
	ApplicationID string    `db:"application_id" insert:"application_id"`
	FromStatus    *string   `db:"from_status"    insert:"from_status"`
	ToStatus      string    `db:"to_status"      insert:"to_status"`
	ActorID       *string   `db:"actor_id"       insert:"actor_id"`
	Reason        *string   `db:"reason"         insert:"reason"`
	CreatedAt     time.Time `db:"created_at"`
}

var (
	creatorApplicationStatusTransitionSelectColumns = sortColumns(stom.MustNewStom(CreatorApplicationStatusTransitionRow{}).SetTag(string(tagSelect)).TagValues())
	creatorApplicationStatusTransitionInsertMapper  = stom.MustNewStom(CreatorApplicationStatusTransitionRow{}).SetTag(string(tagInsert))
)

// CreatorApplicationStatusTransitionRepo lists all public methods of the
// transition repository.
type CreatorApplicationStatusTransitionRepo interface {
	Insert(ctx context.Context, row CreatorApplicationStatusTransitionRow) error
	GetLatestByApplicationAndToStatus(ctx context.Context, applicationID, toStatus string) (*CreatorApplicationStatusTransitionRow, error)
}

type creatorApplicationStatusTransitionRepository struct {
	db dbutil.DB
}

// Insert records a single status transition. id and created_at are filled
// by Postgres defaults, so the input row only carries the business fields.
func (r *creatorApplicationStatusTransitionRepository) Insert(ctx context.Context, row CreatorApplicationStatusTransitionRow) error {
	q := sq.Insert(TableCreatorApplicationStatusTransitions).
		SetMap(toMap(row, creatorApplicationStatusTransitionInsertMapper))
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// GetLatestByApplicationAndToStatus returns the most recent transition row for
// the given application/to-status pair. Multiple matches can exist if the
// state machine ever cycles through the same status more than once; callers
// only care about the latest, so the helper sorts by created_at desc and
// limits to 1. Wrapped sql.ErrNoRows surfaces when no match exists — callers
// (the read aggregate that builds the rejection block) interpret it as "the
// status was reached without a recorded transition" and fall back to a
// log-warn-without-block degradation.
func (r *creatorApplicationStatusTransitionRepository) GetLatestByApplicationAndToStatus(ctx context.Context, applicationID, toStatus string) (*CreatorApplicationStatusTransitionRow, error) {
	q := sq.Select(creatorApplicationStatusTransitionSelectColumns...).
		From(TableCreatorApplicationStatusTransitions).
		Where(sq.Eq{
			CreatorApplicationStatusTransitionColumnApplicationID: applicationID,
			CreatorApplicationStatusTransitionColumnToStatus:      toStatus,
		}).
		OrderBy(CreatorApplicationStatusTransitionColumnCreatedAt + " DESC").
		Limit(1)
	return dbutil.One[CreatorApplicationStatusTransitionRow](ctx, r.db, q)
}
