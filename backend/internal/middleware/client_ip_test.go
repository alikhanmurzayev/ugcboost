package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestClientIP(t *testing.T) {
	t.Parallel()

	t.Run("X-Forwarded-For takes first", func(t *testing.T) {
		t.Parallel()
		router, seen := captureIP(t)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, "1.2.3.4", *seen)
	})

	t.Run("X-Real-IP overrides RemoteAddr", func(t *testing.T) {
		t.Parallel()
		router, seen := captureIP(t)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Real-IP", "9.9.9.9")
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, "9.9.9.9", *seen)
	})

	t.Run("falls back to RemoteAddr without port", func(t *testing.T) {
		t.Parallel()
		router, seen := captureIP(t)
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:5555"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, "10.0.0.1", *seen)
	})
}

func TestWithClientIP(t *testing.T) {
	t.Parallel()

	t.Run("WithClientIP stamps a non-HTTP marker readable by ClientIPFromContext", func(t *testing.T) {
		t.Parallel()
		ctx := WithClientIP(context.Background(), "telegram-bot")
		require.Equal(t, "telegram-bot", ClientIPFromContext(ctx))
	})

	t.Run("WithClientIP overrides earlier value", func(t *testing.T) {
		t.Parallel()
		ctx := WithClientIP(context.Background(), "first")
		ctx = WithClientIP(ctx, "second")
		require.Equal(t, "second", ClientIPFromContext(ctx))
	})
}
