package handler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// logAudit writes an audit entry if svc is non-nil. Logs error but doesn't fail the request.
func logAudit(ctx context.Context, svc AuditLogService, entry service.AuditEntry) {
	if svc == nil {
		return
	}
	if err := svc.Log(ctx, entry); err != nil {
		slog.Error("audit log failed", "error", err, "action", entry.Action)
	}
}

// clientIP extracts the client IP address from the request.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get(HeaderXForwardedFor); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get(HeaderXRealIP); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
