package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecureHeaders_Present(t *testing.T) {
	handler := SecureHeaders(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}

func TestSecureHeaders_DeletesBeforeNext(t *testing.T) {
	var serverInNext, poweredInNext string
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// SecureHeaders calls Del() before next, so these should be empty
		serverInNext = w.Header().Get("Server")
		poweredInNext = w.Header().Get("X-Powered-By")
		w.WriteHeader(http.StatusOK)
	})

	// Pre-set headers on recorder to simulate a reverse proxy
	handler := SecureHeaders(inner)
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	w.Header().Set("Server", "Go")
	w.Header().Set("X-Powered-By", "Go")
	handler.ServeHTTP(w, r)

	assert.Empty(t, serverInNext)
	assert.Empty(t, poweredInNext)
}
