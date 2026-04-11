package handler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// Auditor logs audit events. Optional dependency — nil-safe.
type Auditor interface {
	Log(ctx context.Context, entry service.AuditEntry) error
}

// logAudit writes an audit entry if auditor is non-nil. Logs error but doesn't fail the request.
func logAudit(ctx context.Context, auditor Auditor, entry service.AuditEntry) {
	if auditor == nil {
		return
	}
	if err := auditor.Log(ctx, entry); err != nil {
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
