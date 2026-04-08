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
	List(ctx context.Context, f repository.AuditFilter, page, perPage int) ([]repository.AuditLogRow, int64, error)
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
	if role != "admin" {
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

	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
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
