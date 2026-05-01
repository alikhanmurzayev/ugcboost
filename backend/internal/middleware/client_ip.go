package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// HTTP header names used by the client-IP resolution chain.
const (
	HeaderCFConnectingIP = "CF-Connecting-IP"
	HeaderTrueClientIP   = "True-Client-IP"
	HeaderXRealIP        = "X-Real-IP"
	HeaderXForwardedFor  = "X-Forwarded-For"
)

// clientIPSingleHeaders are scanned in order; the first header that contains
// a parseable IP wins. X-Forwarded-For is handled separately because its
// value is a comma-separated chain and we want only the leftmost client IP.
//
// Trust grade differs across the chain: CF-Connecting-IP and True-Client-IP
// are set by the Cloudflare edge itself and overwrite anything the client
// supplies; X-Real-IP and X-Forwarded-For are populated by Dokploy's reverse
// proxy and are best-effort fallbacks for traffic not fronted by Cloudflare
// (local dev, direct-to-origin tests).
var clientIPSingleHeaders = [3]string{
	HeaderCFConnectingIP,
	HeaderTrueClientIP,
	HeaderXRealIP,
}

// maxIPTextLen caps the per-header text we feed into net.ParseIP. The longest
// valid IPv6 textual form is 39 characters; 45 leaves slack for surrounding
// whitespace without inviting a tight DoS loop on attacker-supplied buffers.
const maxIPTextLen = 45

type clientIPKey struct{}

// RealIP rewrites r.RemoteAddr with the real client IP resolved from the
// proxy-chain headers. Priority: CF-Connecting-IP → True-Client-IP →
// X-Real-IP → X-Forwarded-For (leftmost) → original r.RemoteAddr. Each
// candidate is validated with net.ParseIP; invalid values are skipped,
// never blocking the next candidate. Per-candidate text is bounded at
// maxIPTextLen so attacker-supplied jumbo headers do not turn into a
// micro-DoS in net.ParseIP.
//
// Trust model: the backend only accepts traffic via Cloudflare → Dokploy →
// Docker. The prod iptables firewall restricts ingress to Cloudflare ranges,
// and inside the Docker network only the Dokploy reverse proxy reaches the
// backend. Header spoofing is therefore prevented at the network layer.
// CF-Connecting-IP and True-Client-IP are authoritative because Cloudflare
// stamps them at the edge and overwrites client-supplied values; X-Real-IP
// and X-Forwarded-For are populated by Dokploy and used as best-effort
// fallbacks (local dev, direct-to-origin tests) — for CF-fronted traffic
// they should never be the deciding source.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := extractClientIP(r); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

func extractClientIP(r *http.Request) string {
	for _, h := range clientIPSingleHeaders {
		if ip := parseHeaderIP(r.Header.Get(h)); ip != "" {
			return ip
		}
	}
	if xff := r.Header.Get(HeaderXForwardedFor); xff != "" {
		// Iterate every comma-separated token: dirty proxies sometimes emit
		// leading empty entries (", , 1.2.3.4") so the leftmost-non-empty
		// must win, not the literal first token.
		for _, raw := range strings.Split(xff, ",") {
			if ip := parseHeaderIP(raw); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// parseHeaderIP trims and validates an IP candidate from a proxy header.
// Accepts both bare IP and `host:port` (RFC 7239 forbids the suffix, but
// dirty proxies emit it) — the host part is what we care about. Returns
// "" for empty, oversized, or unparseable inputs so callers can fall
// through to the next priority source.
func parseHeaderIP(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > maxIPTextLen {
		return ""
	}
	if net.ParseIP(v) != nil {
		return v
	}
	if host, _, err := net.SplitHostPort(v); err == nil && net.ParseIP(host) != nil {
		return host
	}
	return ""
}

// ClientIP stores the client IP (already normalised by RealIP) on the
// request context so strict-server handlers and downstream services can
// read it without touching *http.Request. Pair it with RealIP — RealIP
// must run first so r.RemoteAddr already holds the resolved IP.
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

// ClientIPFromContext returns the client IP previously stored by ClientIP.
func ClientIPFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clientIPKey{}).(string)
	return v
}

// WithClientIP attaches an explicit client IP to the context. Used by
// non-HTTP code paths (background workers, the Telegram bot) so audit_logs
// rows still carry an honest, non-empty marker.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}
