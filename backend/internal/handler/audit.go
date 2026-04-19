package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// ListAuditLogs handles GET /audit-logs
func (s *Server) ListAuditLogs(w http.ResponseWriter, r *http.Request, params api.ListAuditLogsParams) {
	if err := s.authzService.CanListAuditLogs(r.Context()); err != nil {
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

	items := make([]api.AuditLogEntry, len(logs))
	for i, l := range logs {
		items[i] = api.AuditLogEntry{
			Id:         l.ID,
			ActorId:    l.ActorID,
			ActorRole:  l.ActorRole,
			Action:     l.Action,
			EntityType: l.EntityType,
			EntityId:   l.EntityID,
			OldValue:   rawJSONToAny(l.ID, l.OldValue),
			NewValue:   rawJSONToAny(l.ID, l.NewValue),
			IpAddress:  l.IPAddress,
			CreatedAt:  l.CreatedAt,
		}
	}

	respondJSON(w, r, http.StatusOK, api.AuditLogsResult{
		Data: api.ListAuditLogsData{
			Logs:    items,
			Page:    page,
			PerPage: perPage,
			Total:   int(total),
		},
	})
}

// rawJSONToAny converts json.RawMessage to any for API serialization.
// On decode failure we log the error with the audit entry context —
// silently dropping it would hide real data corruption.
func rawJSONToAny(id string, raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		slog.Error("failed to unmarshal audit log value", "error", err, "auditLogID", id)
		return nil
	}
	return v
}
