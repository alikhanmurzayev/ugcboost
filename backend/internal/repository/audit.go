package repository

import (
	"context"
	"encoding/json"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// AuditLogs table and column names.
const (
	TableAuditLogs              = "audit_logs"
	AuditLogColumnID            = "id"
	AuditLogColumnActorID       = "actor_id"
	AuditLogColumnActorRole     = "actor_role"
	AuditLogColumnAction        = "action"
	AuditLogColumnEntityType    = "entity_type"
	AuditLogColumnEntityID      = "entity_id"
	AuditLogColumnOldValue      = "old_value"
	AuditLogColumnNewValue      = "new_value"
	AuditLogColumnIPAddress     = "ip_address"
	AuditLogColumnCreatedAt     = "created_at"
)

// AuditLogRow maps to the audit_logs table.
type AuditLogRow struct {
	ID         string          `db:"id"`
	ActorID    string          `db:"actor_id"     insert:"actor_id"`
	ActorRole  string          `db:"actor_role"   insert:"actor_role"`
	Action     string          `db:"action"       insert:"action"`
	EntityType string          `db:"entity_type"  insert:"entity_type"`
	EntityID   *string         `db:"entity_id"    insert:"entity_id"`
	OldValue   json.RawMessage `db:"old_value"    insert:"old_value"`
	NewValue   json.RawMessage `db:"new_value"    insert:"new_value"`
	IPAddress  string          `db:"ip_address"   insert:"ip_address"`
	CreatedAt  time.Time       `db:"created_at"`
}

var (
	auditSelectColumns = sortColumns(stom.MustNewStom(AuditLogRow{}).SetTag(string(tagSelect)).TagValues())
	auditInsertMapper  = stom.MustNewStom(AuditLogRow{}).SetTag(string(tagInsert))
	auditInsertColumns = sortColumns(auditInsertMapper.TagValues())
)

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
		Insert(TableAuditLogs).
		SetMap(toMap(entry, auditInsertMapper))

	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// List returns audit logs matching the given filter with pagination.
func (r *AuditRepository) List(ctx context.Context, f AuditFilter, page, perPage int) ([]AuditLogRow, int64, error) {
	// Count
	countQ := dbutil.Psql.Select("COUNT(*)").From(TableAuditLogs)
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
		Select(auditSelectColumns...).
		From(TableAuditLogs)

	q = applyAuditFilters(q, f)
	q = q.OrderBy(AuditLogColumnCreatedAt + " DESC")

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
		q = q.Where(sq.Eq{AuditLogColumnActorID: f.ActorID})
	}
	if f.EntityType != "" {
		q = q.Where(sq.Eq{AuditLogColumnEntityType: f.EntityType})
	}
	if f.EntityID != "" {
		q = q.Where(sq.Eq{AuditLogColumnEntityID: f.EntityID})
	}
	if f.Action != "" {
		q = q.Where(sq.Eq{AuditLogColumnAction: f.Action})
	}
	if f.DateFrom != nil {
		q = q.Where(sq.GtOrEq{AuditLogColumnCreatedAt: *f.DateFrom})
	}
	if f.DateTo != nil {
		q = q.Where(sq.LtOrEq{AuditLogColumnCreatedAt: *f.DateTo})
	}
	return q
}
