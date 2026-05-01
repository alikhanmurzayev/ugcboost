package middleware

import (
	"context"
	"net/http"
)

type userAgentKey struct{}

// MaxUserAgentLength caps the User-Agent before any downstream consumer reads
// it. Attacker-controlled headers must not balloon DB rows or logs.
const MaxUserAgentLength = 1024

// RequestMeta truncates and stores the User-Agent on ctx so strict-server
// handlers (no `*http.Request` access) can read it.
func RequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		if len(ua) > MaxUserAgentLength {
			ua = ua[:MaxUserAgentLength]
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userAgentKey{}, ua)))
	})
}

// UserAgentFromContext returns the User-Agent stored by RequestMeta, or "".
func UserAgentFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userAgentKey{}).(string)
	return v
}

// WithUserAgent attaches an explicit User-Agent to ctx (test seam).
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, ua)
}
