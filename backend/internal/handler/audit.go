package handler

import (
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// ListAuditLogs handles GET /audit-logs
func (s *Server) ListAuditLogs(w http.ResponseWriter, r *http.Request, params api.ListAuditLogsParams) {
	if err := authz.RequireAdmin(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}

	f := domain.AuditFilter{}
	if params.ActorId != nil {
		f.ActorID = *params.ActorId
	}
	if params.EntityType != nil {
		f.EntityType = *params.EntityType
	}
	if params.EntityId != nil {
		f.EntityID = *params.EntityId
	}
	if params.Action != nil {
		f.Action = *params.Action
	}
	if params.DateFrom != nil {
		f.DateFrom = params.DateFrom
	}
	if params.DateTo != nil {
		f.DateTo = params.DateTo
	}

	page := 1
	if params.Page != nil {
		page = *params.Page
	}
	perPage := 20
	if params.PerPage != nil {
		perPage = *params.PerPage
	}

	logs, total, err := s.auditService.List(r.Context(), f, page, perPage)
	if err != nil {
		respondError(w, r, err)
		return
	}

	items := make([]map[string]any, len(logs))
	for i, l := range logs {
		items[i] = map[string]any{
			"id":         l.ID,
			"actorId":    l.ActorID,
			"actorRole":  l.ActorRole,
			"action":     l.Action,
			"entityType": l.EntityType,
			"entityId":   l.EntityID,
			"oldValue":   l.OldValue,
			"newValue":   l.NewValue,
			"ipAddress":  l.IPAddress,
			"createdAt":  l.CreatedAt,
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"logs":    items,
		"total":   total,
		"page":    page,
		"perPage": perPage,
	})
}
