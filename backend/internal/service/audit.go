package service

import (
	"context"
	"encoding/json"
	"fmt"

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

// Log writes an audit log entry. Returns error so the caller can decide how to handle it.
func (s *AuditService) Log(ctx context.Context, e AuditEntry) error {
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
		data, err := json.Marshal(e.OldValue)
		if err != nil {
			return fmt.Errorf("marshal old_value: %w", err)
		}
		row.OldValue = data
	}
	if e.NewValue != nil {
		data, err := json.Marshal(e.NewValue)
		if err != nil {
			return fmt.Errorf("marshal new_value: %w", err)
		}
		row.NewValue = data
	}

	return s.repo.Create(ctx, row)
}

// List returns audit logs matching the filter with pagination.
// Validation is the handler's responsibility (CS-18).
func (s *AuditService) List(ctx context.Context, f repository.AuditFilter, page, perPage int) ([]repository.AuditLogRow, int64, error) {
	return s.repo.List(ctx, f, page, perPage)
}
