package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// AuditLogs is the interface AuditHandler needs from the audit service.
type AuditLogs interface {
	List(ctx context.Context, f repository.AuditFilter, page, perPage int) ([]*repository.AuditLogRow, int64, error)
}

// AuditHandler handles audit log endpoints.
type AuditHandler struct {
	audit AuditLogs
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(audit AuditLogs) *AuditHandler {
	return &AuditHandler{audit: audit}
}

// ListAuditLogs handles GET /api/audit-logs
func (h *AuditHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	if role != string(domain.RoleAdmin) {
		respondError(w, r, domain.ErrForbidden)
		return
	}

	q := r.URL.Query()

	f := repository.AuditFilter{
		ActorID:    q.Get("actor_id"),
		EntityType: q.Get("entity_type"),
		EntityID:   q.Get("entity_id"),
		Action:     q.Get("action"),
	}

	if v := q.Get("date_from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			f.DateFrom = &t
		}
	}
	if v := q.Get("date_to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			f.DateTo = &t
		}
	}

	page := 1
	if v := q.Get("page"); v != "" {
		var err error
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 {
			respondError(w, r, domain.NewValidationError(domain.CodeValidation, "page must be a positive integer"))
			return
		}
	}
	perPage := 20
	if v := q.Get("per_page"); v != "" {
		var err error
		perPage, err = strconv.Atoi(v)
		if err != nil || perPage < 1 || perPage > 100 {
			respondError(w, r, domain.NewValidationError(domain.CodeValidation, "per_page must be between 1 and 100"))
			return
		}
	}

	logs, total, err := h.audit.List(r.Context(), f, page, perPage)
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
