package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecovery_NoPanic(t *testing.T) {
	handler := Recovery(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery_PanicReturns500(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("something broke")
	}))

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	resp := parseError(t, w)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestRecovery_PanicWithError(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(assert.AnError)
	}))

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
