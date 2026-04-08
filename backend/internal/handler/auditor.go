package handler

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// Auditor logs audit events. Optional dependency — nil-safe.
type Auditor interface {
	Log(ctx context.Context, entry service.AuditEntry)
}

// clientIP extracts the client IP address from the request.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
