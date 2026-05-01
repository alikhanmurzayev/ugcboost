package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// captureIP returns a handler chain that records the IP seen by
// the inner handler after RealIP and ClientIP middleware ran.
func captureIP(t *testing.T) (chi.Router, *string) {
	t.Helper()
	var seen string
	r := chi.NewRouter()
	r.Use(RealIP)
	r.Use(ClientIP)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		seen = ClientIPFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	return r, &seen
}

func TestRealIP(t *testing.T) {
	t.Parallel()

	type headers struct {
		cfConnectingIP string
		trueClientIP   string
		xRealIP        string
		xForwardedFor  string
	}

	tests := []struct {
		name       string
		hdr        headers
		remoteAddr string
		want       string
	}{
		{
			name:       "CF-Connecting-IP wins over X-Forwarded-For",
			hdr:        headers{cfConnectingIP: "203.0.113.7", xForwardedFor: "1.2.3.4"},
			remoteAddr: "172.18.0.5:33000",
			want:       "203.0.113.7",
		},
		{
			name:       "True-Client-IP if no CF",
			hdr:        headers{trueClientIP: "5.6.7.8", xForwardedFor: "1.2.3.4"},
			remoteAddr: "172.18.0.5:33000",
			want:       "5.6.7.8",
		},
		{
			name:       "X-Real-IP if no CF and no True-Client-IP",
			hdr:        headers{xRealIP: "9.9.9.9"},
			remoteAddr: "127.0.0.1:1234",
			want:       "9.9.9.9",
		},
		{
			name:       "X-Forwarded-For leftmost",
			hdr:        headers{xForwardedFor: "1.2.3.4, 5.6.7.8"},
			remoteAddr: "127.0.0.1:1234",
			want:       "1.2.3.4",
		},
		{
			name:       "skip invalid CF, fall to valid X-Real-IP",
			hdr:        headers{cfConnectingIP: "garbage", xRealIP: "9.9.9.9"},
			remoteAddr: "127.0.0.1:1234",
			want:       "9.9.9.9",
		},
		{
			name:       "all headers invalid falls back to RemoteAddr",
			hdr:        headers{cfConnectingIP: "not-an-ip", xRealIP: "bogus", xForwardedFor: "still-bad"},
			remoteAddr: "10.0.0.1:5555",
			want:       "10.0.0.1",
		},
		{
			name:       "oversized header value is skipped",
			hdr:        headers{cfConnectingIP: strings.Repeat("a", maxIPTextLen+1), xRealIP: "9.9.9.9"},
			remoteAddr: "127.0.0.1:1234",
			want:       "9.9.9.9",
		},
		{
			name:       "X-Forwarded-For with leading empty tokens",
			hdr:        headers{xForwardedFor: ", , 1.2.3.4"},
			remoteAddr: "127.0.0.1:1234",
			want:       "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For with host:port token",
			hdr:        headers{xForwardedFor: "1.2.3.4:5678, 5.6.7.8"},
			remoteAddr: "127.0.0.1:1234",
			want:       "1.2.3.4",
		},
		{
			name:       "CF-Connecting-IP with host:port",
			hdr:        headers{cfConnectingIP: "203.0.113.7:5555"},
			remoteAddr: "127.0.0.1:1234",
			want:       "203.0.113.7",
		},
		{
			name:       "no headers falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:5555",
			want:       "10.0.0.1",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "10.0.0.1",
			want:       "10.0.0.1",
		},
		{
			name:       "IPv6 in CF-Connecting-IP",
			hdr:        headers{cfConnectingIP: "2001:db8::1"},
			remoteAddr: "[::1]:443",
			want:       "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			router, seen := captureIP(t)
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.hdr.cfConnectingIP != "" {
				req.Header.Set(HeaderCFConnectingIP, tt.hdr.cfConnectingIP)
			}
			if tt.hdr.trueClientIP != "" {
				req.Header.Set(HeaderTrueClientIP, tt.hdr.trueClientIP)
			}
			if tt.hdr.xRealIP != "" {
				req.Header.Set(HeaderXRealIP, tt.hdr.xRealIP)
			}
			if tt.hdr.xForwardedFor != "" {
				req.Header.Set(HeaderXForwardedFor, tt.hdr.xForwardedFor)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tt.want, *seen)
		})
	}
}

func TestWithClientIP(t *testing.T) {
	t.Parallel()

	t.Run("stamps marker readable by ClientIPFromContext", func(t *testing.T) {
		t.Parallel()
		ctx := WithClientIP(context.Background(), "telegram-bot")
		require.Equal(t, "telegram-bot", ClientIPFromContext(ctx))
	})

	t.Run("overrides earlier value", func(t *testing.T) {
		t.Parallel()
		ctx := WithClientIP(context.Background(), "first")
		ctx = WithClientIP(ctx, "second")
		require.Equal(t, "second", ClientIPFromContext(ctx))
	})
}
