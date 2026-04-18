package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// AuditRepoFactory creates repositories needed by AuditService.
type AuditRepoFactory interface {
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

func domainFilterToRepo(f domain.AuditFilter) repository.AuditFilter {
	return repository.AuditFilter{
		ActorID:    f.ActorID,
		EntityType: f.EntityType,
		EntityID:   f.EntityID,
		Action:     f.Action,
		DateFrom:   f.DateFrom,
		DateTo:     f.DateTo,
	}
}

func auditRowsToDomain(rows []*repository.AuditLogRow) []*domain.AuditLog {
	result := make([]*domain.AuditLog, len(rows))
	for i, row := range rows {
		result[i] = &domain.AuditLog{
			ID:         row.ID,
			ActorID:    row.ActorID,
			ActorRole:  row.ActorRole,
			Action:     row.Action,
			EntityType: row.EntityType,
			EntityID:   row.EntityID,
			OldValue:   row.OldValue,
			NewValue:   row.NewValue,
			IPAddress:  row.IPAddress,
			CreatedAt:  row.CreatedAt,
		}
	}
	return result
}

// AuditService handles audit logging.
type AuditService struct {
	pool        dbutil.Pool
	repoFactory AuditRepoFactory
}

// NewAuditService creates a new AuditService.
func NewAuditService(pool dbutil.Pool, repoFactory AuditRepoFactory) *AuditService {
	return &AuditService{pool: pool, repoFactory: repoFactory}
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
	auditRepo := s.repoFactory.NewAuditRepo(s.pool)

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

	return auditRepo.Create(ctx, row)
}

// List returns audit logs matching the filter with pagination.
// Validation is the handler's responsibility (CS-18).
func (s *AuditService) List(ctx context.Context, f domain.AuditFilter, page, perPage int) ([]*domain.AuditLog, int64, error) {
	auditRepo := s.repoFactory.NewAuditRepo(s.pool)
	rows, total, err := auditRepo.List(ctx, domainFilterToRepo(f), page, perPage)
	if err != nil {
		return nil, 0, err
	}
	return auditRowsToDomain(rows), total, nil
}
