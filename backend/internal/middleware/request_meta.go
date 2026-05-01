package middleware

import (
	"context"
	"net/http"
)

type userAgentKey struct{}
type refreshCookieKey struct{}

// CookieRefreshToken is the name of the httpOnly cookie that carries the
// refresh token. Re-declared here (mirrored in handler.CookieRefreshToken) so
// middleware does not import handler — avoids an import cycle.
const CookieRefreshToken = "refresh_token"

// RequestMeta captures *http.Request fields that strict-server-mode handlers
// can no longer read directly (the strict signature is `(ctx, request)` —
// no ResponseWriter, no Request). Currently surfaces:
//   - User-Agent (raw, untruncated; consumers truncate as needed)
//   - refresh_token cookie value (empty if absent)
func RequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userAgentKey{}, r.UserAgent())
		if c, err := r.Cookie(CookieRefreshToken); err == nil && c.Value != "" {
			ctx = context.WithValue(ctx, refreshCookieKey{}, c.Value)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserAgentFromContext returns the User-Agent header captured by RequestMeta,
// or the empty string if none was present / RequestMeta was not mounted.
func UserAgentFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userAgentKey{}).(string)
	return v
}

// RefreshCookieFromContext returns the refresh_token cookie value captured by
// RequestMeta, or the empty string if absent.
func RefreshCookieFromContext(ctx context.Context) string {
	v, _ := ctx.Value(refreshCookieKey{}).(string)
	return v
}

// WithUserAgent attaches an explicit User-Agent to the context. Tests use this
// to seed the value RequestMeta would normally extract from the HTTP request.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, ua)
}

// WithRefreshCookie attaches an explicit refresh cookie to the context. Tests
// use this to bypass the cookie-extraction step performed by RequestMeta.
func WithRefreshCookie(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, refreshCookieKey{}, raw)
}
