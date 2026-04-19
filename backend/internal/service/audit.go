package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// contextWithActor returns a copy of ctx carrying userID and role,
// so writeAudit can read consistent actor data for operations (like Login)
// where the authenticated user is not yet present in the incoming request context.
func contextWithActor(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
	return context.WithValue(ctx, middleware.ContextKeyRole, api.UserRole(role))
}

// writeAudit appends a single audit record using the supplied tx-bound repo.
// Actor data (user id, role, ip) is pulled from the request context so that
// every service call-site stays consistent. EntityID may be empty ("") for
// events that do not target a specific entity.
func writeAudit(ctx context.Context, repo repository.AuditRepo, action, entityType, entityID string, oldValue, newValue any) error {
	row := repository.AuditLogRow{
		ActorID:    middleware.UserIDFromContext(ctx),
		ActorRole:  string(middleware.RoleFromContext(ctx)),
		Action:     action,
		EntityType: entityType,
		IPAddress:  middleware.ClientIPFromContext(ctx),
	}
	if entityID != "" {
		id := entityID
		row.EntityID = &id
	}
	if oldValue != nil {
		data, err := json.Marshal(oldValue)
		if err != nil {
			return fmt.Errorf("marshal old_value: %w", err)
		}
		row.OldValue = data
	}
	if newValue != nil {
		data, err := json.Marshal(newValue)
		if err != nil {
			return fmt.Errorf("marshal new_value: %w", err)
		}
		row.NewValue = data
	}
	return repo.Create(ctx, row)
}

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

// AuditService exposes read access to the audit log. Writes happen inside
// the services that own the mutating operation, within the same transaction.
type AuditService struct {
	pool        dbutil.Pool
	repoFactory AuditRepoFactory
}

// NewAuditService creates a new AuditService.
func NewAuditService(pool dbutil.Pool, repoFactory AuditRepoFactory) *AuditService {
	return &AuditService{pool: pool, repoFactory: repoFactory}
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
