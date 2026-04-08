package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// AuditRepo is the interface AuditService needs from the audit repository.
type AuditRepo interface {
	Create(ctx context.Context, entry repository.AuditLogRow) error
	List(ctx context.Context, f repository.AuditFilter, page, perPage int) ([]repository.AuditLogRow, int64, error)
}

// AuditService handles audit logging.
type AuditService struct {
	repo AuditRepo
}

// NewAuditService creates a new AuditService.
func NewAuditService(repo AuditRepo) *AuditService {
	return &AuditService{repo: repo}
}

// AuditEntry describes a single audit event to record.
type AuditEntry struct {
	ActorID    string
	ActorRole  string
	Action     string
	EntityType string
	EntityID   string
	OldValue   any
	NewValue   any
	IPAddress  string
}

// Log writes an audit log entry. Never fails visibly — errors are logged.
func (s *AuditService) Log(ctx context.Context, e AuditEntry) {
	var entityID *string
	if e.EntityID != "" {
		entityID = &e.EntityID
	}

	row := repository.AuditLogRow{
		ActorID:    e.ActorID,
		ActorRole:  e.ActorRole,
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   entityID,
		IPAddress:  e.IPAddress,
	}

	if e.OldValue != nil {
		if data, err := json.Marshal(e.OldValue); err != nil {
			slog.Warn("audit: marshal old_value failed", "error", err, "action", e.Action)
		} else {
			row.OldValue = data
		}
	}
	if e.NewValue != nil {
		if data, err := json.Marshal(e.NewValue); err != nil {
			slog.Warn("audit: marshal new_value failed", "error", err, "action", e.Action)
		} else {
			row.NewValue = data
		}
	}

	if err := s.repo.Create(ctx, row); err != nil {
		slog.Error("audit log failed", "error", err, "action", e.Action)
	}
}

// List returns audit logs matching the filter with pagination.
func (s *AuditService) List(ctx context.Context, f repository.AuditFilter, page, perPage int) ([]repository.AuditLogRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return s.repo.List(ctx, f, page, perPage)
}
