package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshCookie(t *testing.T) {
	t.Parallel()

	t.Run("attaches cookie on /auth/refresh", func(t *testing.T) {
		t.Parallel()
		var seen string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "raw-token"})

		RefreshCookie(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, "raw-token", seen)
	})

	t.Run("attaches cookie on /auth/logout", func(t *testing.T) {
		t.Parallel()
		var seen string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "raw-token"})

		RefreshCookie(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, "raw-token", seen)
	})

	t.Run("does not attach cookie on unrelated route", func(t *testing.T) {
		t.Parallel()
		var seen string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: "raw-token"})

		RefreshCookie(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, seen)
	})

	t.Run("missing cookie on whitelisted route yields empty", func(t *testing.T) {
		t.Parallel()
		var seen string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		RefreshCookie(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, seen)
	})

	t.Run("empty cookie value treated as missing", func(t *testing.T) {
		t.Parallel()
		var seen string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RefreshCookieFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: CookieRefreshToken, Value: ""})

		RefreshCookie(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, seen)
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
