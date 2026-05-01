package middleware

import (
	"context"
	"net/http"
)

type refreshCookieKey struct{}

const CookieRefreshToken = "refresh_token"

// refreshCookieRoutes is the closed list of paths that legitimately need the
// raw refresh-token surfaced on ctx. Mounting RefreshCookie globally but only
// touching these paths keeps the secret out of every other request's ctx —
// otherwise any logging/recovery middleware that dumps ctx leaks it.
var refreshCookieRoutes = map[string]struct{}{
	"/auth/refresh": {},
	"/auth/logout":  {},
}

// RefreshCookie places the refresh_token cookie on ctx for the routes listed in
// refreshCookieRoutes only. Other requests pass through untouched.
func RefreshCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := refreshCookieRoutes[r.URL.Path]; !ok {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(CookieRefreshToken)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), refreshCookieKey{}, c.Value)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RefreshCookieFromContext returns the refresh-token cookie attached by RefreshCookie, or "".
func RefreshCookieFromContext(ctx context.Context) string {
	v, _ := ctx.Value(refreshCookieKey{}).(string)
	return v
}

// WithRefreshCookie attaches an explicit refresh cookie to ctx (test seam).
func WithRefreshCookie(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, refreshCookieKey{}, raw)
}
