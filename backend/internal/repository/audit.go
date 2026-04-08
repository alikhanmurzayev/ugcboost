package repository

import (
	"context"
	"encoding/json"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// AuditLogRow maps to the audit_logs table.
type AuditLogRow struct {
	ID         string          `db:"id"`
	ActorID    string          `db:"actor_id"`
	ActorRole  string          `db:"actor_role"`
	Action     string          `db:"action"`
	EntityType string          `db:"entity_type"`
	EntityID   *string         `db:"entity_id"`
	OldValue   json.RawMessage `db:"old_value"`
	NewValue   json.RawMessage `db:"new_value"`
	IPAddress  string          `db:"ip_address"`
	CreatedAt  time.Time       `db:"created_at"`
}

// AuditFilter defines the filter parameters for listing audit logs.
type AuditFilter struct {
	ActorID    string
	EntityType string
	EntityID   string
	Action     string
	DateFrom   *time.Time
	DateTo     *time.Time
}

// AuditRepository handles audit log persistence.
type AuditRepository struct {
	db dbutil.DB
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db dbutil.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// Create inserts a new audit log entry.
func (r *AuditRepository) Create(ctx context.Context, entry AuditLogRow) error {
	q := dbutil.Psql.
		Insert("audit_logs").
		Columns("actor_id", "actor_role", "action", "entity_type", "entity_id", "old_value", "new_value", "ip_address").
		Values(entry.ActorID, entry.ActorRole, entry.Action, entry.EntityType, entry.EntityID, entry.OldValue, entry.NewValue, entry.IPAddress)

	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// List returns audit logs matching the given filter with pagination.
func (r *AuditRepository) List(ctx context.Context, f AuditFilter, page, perPage int) ([]AuditLogRow, int64, error) {
	// Count
	countQ := dbutil.Psql.Select("COUNT(*)").From("audit_logs")
	countQ = applyAuditFilters(countQ, f)
	total, err := dbutil.Val[int64](ctx, r.db, countQ)
	if err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return nil, 0, nil
	}

	// Data
	q := dbutil.Psql.
		Select("id", "actor_id", "actor_role", "action", "entity_type", "entity_id", "old_value", "new_value", "ip_address", "created_at").
		From("audit_logs")

	q = applyAuditFilters(q, f)
	q = q.OrderBy("created_at DESC")

	offset := (page - 1) * perPage
	q = q.Limit(uint64(perPage)).Offset(uint64(offset))

	rows, err := dbutil.Many[AuditLogRow](ctx, r.db, q)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

func applyAuditFilters(q sq.SelectBuilder, f AuditFilter) sq.SelectBuilder {
	if f.ActorID != "" {
		q = q.Where(sq.Eq{"actor_id": f.ActorID})
	}
	if f.EntityType != "" {
		q = q.Where(sq.Eq{"entity_type": f.EntityType})
	}
	if f.EntityID != "" {
		q = q.Where(sq.Eq{"entity_id": f.EntityID})
	}
	if f.Action != "" {
		q = q.Where(sq.Eq{"action": f.Action})
	}
	if f.DateFrom != nil {
		q = q.Where(sq.GtOrEq{"created_at": *f.DateFrom})
	}
	if f.DateTo != nil {
		q = q.Where(sq.LtOrEq{"created_at": *f.DateTo})
	}
	return q
}
