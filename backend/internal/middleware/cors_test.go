package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCORS_Origin(t *testing.T) {
	t.Parallel()

	t.Run("allowed origin", func(t *testing.T) {
		t.Parallel()
		handler := CORS([]string{"http://localhost:5173"})(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		require.Equal(t, "Origin", w.Header().Get("Vary"))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disallowed origin", func(t *testing.T) {
		t.Parallel()
		handler := CORS([]string{"http://localhost:5173"})(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Origin", "http://evil.com")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("preflight allowed", func(t *testing.T) {
		t.Parallel()
		handler := CORS([]string{"http://localhost:5173"})(okHandler())

		r := httptest.NewRequest("OPTIONS", "/", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusNoContent, w.Code)
		require.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
		require.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	})

	t.Run("preflight denied", func(t *testing.T) {
		t.Parallel()
		handler := CORS([]string{"http://localhost:5173"})(okHandler())

		r := httptest.NewRequest("OPTIONS", "/", nil)
		r.Header.Set("Origin", "http://evil.com")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("no origin", func(t *testing.T) {
		t.Parallel()
		handler := CORS([]string{"http://localhost:5173"})(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, http.StatusOK, w.Code)
	})
}
