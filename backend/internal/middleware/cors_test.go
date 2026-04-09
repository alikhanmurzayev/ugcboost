package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORS_AllowedOrigin(t *testing.T) {
	t.Parallel()
	handler := CORS([]string{"http://localhost:5173"})(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "Origin", w.Header().Get("Vary"))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	t.Parallel()
	handler := CORS([]string{"http://localhost:5173"})(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCORS_PreflightAllowed(t *testing.T) {
	t.Parallel()
	handler := CORS([]string{"http://localhost:5173"})(okHandler())

	r := httptest.NewRequest("OPTIONS", "/", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
}

func TestCORS_PreflightDenied(t *testing.T) {
	t.Parallel()
	handler := CORS([]string{"http://localhost:5173"})(okHandler())

	r := httptest.NewRequest("OPTIONS", "/", nil)
	r.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCORS_NoOrigin(t *testing.T) {
	t.Parallel()
	handler := CORS([]string{"http://localhost:5173"})(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, w.Code)
}
