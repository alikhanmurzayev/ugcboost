package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestMeta(t *testing.T) {
	t.Parallel()

	t.Run("captures user agent and refresh cookie", func(t *testing.T) {
		t.Parallel()
		var (
			seenUA     string
			seenCookie string
		)
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenUA = UserAgentFromContext(r.Context())
			seenCookie = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "raw-token"})

		RequestMeta(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, "Mozilla/5.0", seenUA)
		require.Equal(t, "raw-token", seenCookie)
	})

	t.Run("missing cookie yields empty value", func(t *testing.T) {
		t.Parallel()
		var seenCookie string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenCookie = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		RequestMeta(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, seenCookie)
	})

	t.Run("empty cookie value treated as missing", func(t *testing.T) {
		t.Parallel()
		var seenCookie string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenCookie = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: ""})

		RequestMeta(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, seenCookie)
	})
}

func TestUserAgentFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns stored value", func(t *testing.T) {
		t.Parallel()
		ctx := WithUserAgent(context.Background(), "explicit-ua")
		require.Equal(t, "explicit-ua", UserAgentFromContext(ctx))
	})

	t.Run("returns empty when absent", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, UserAgentFromContext(context.Background()))
	})
}

func TestRefreshCookieFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns stored value", func(t *testing.T) {
		t.Parallel()
		ctx := WithRefreshCookie(context.Background(), "explicit-token")
		require.Equal(t, "explicit-token", RefreshCookieFromContext(ctx))
	})

	t.Run("returns empty when absent", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, RefreshCookieFromContext(context.Background()))
	})
}
