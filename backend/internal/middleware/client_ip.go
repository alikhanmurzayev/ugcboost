package middleware

import (
	"context"
	"net"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type clientIPKey struct{}

// ClientIP stores the client IP (from r.RemoteAddr, as normalised by
// chi/middleware.RealIP) in the request context. Pair it with
// chimw.RealIP so trusted proxy headers (X-Forwarded-For, X-Real-IP)
// populate r.RemoteAddr before this middleware reads it.
func ClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		ctx := context.WithValue(r.Context(), clientIPKey{}, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RealIP re-exports chi's middleware so callers do not import chi in wiring code.
var RealIP = chimw.RealIP

// ClientIPFromContext returns the client IP previously stored by ClientIP.
func ClientIPFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clientIPKey{}).(string)
	return v
}
