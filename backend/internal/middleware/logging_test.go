package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func argsAsString(args []any) string {
	return fmt.Sprint(args...)
}

func captureInfoArgs(log *logmocks.MockLogger, msg string, dst *[]any, mu *sync.Mutex) {
	log.EXPECT().Info(mock.Anything, msg, mock.Anything).Run(func(_ context.Context, _ string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		*dst = append([]any(nil), args...)
	}).Once()
}

func TestLogging_LogsRequest(t *testing.T) {
	t.Parallel()

	log := logmocks.NewMockLogger(t)
	var mu sync.Mutex
	var captured []any
	captureInfoArgs(log, "http request", &captured, &mu)

	handler := Logging(log)(okHandler())

	r := httptest.NewRequest("GET", "/foo", nil)
	r.RemoteAddr = "127.0.0.1:4242"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	require.Len(t, captured, 10, "expected 5 key/value pairs")
	require.Equal(t, "method", captured[0])
	require.Equal(t, "GET", captured[1])
	require.Equal(t, "path", captured[2])
	require.Equal(t, "/foo", captured[3])
	require.Equal(t, "status", captured[4])
	require.Equal(t, 200, captured[5])
	require.Equal(t, "duration_ms", captured[6])
	require.IsType(t, int64(0), captured[7])
	require.Equal(t, "remote_addr", captured[8])
	require.Equal(t, "127.0.0.1:4242", captured[9])
}

func TestLogging_DoesNotLogAuthorization(t *testing.T) {
	t.Parallel()

	log := logmocks.NewMockLogger(t)
	var mu sync.Mutex
	var captured []any
	captureInfoArgs(log, "http request", &captured, &mu)

	handler := Logging(log)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer super-secret-jwt-token-value")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	s := argsAsString(captured)
	require.NotContains(t, s, "super-secret-jwt-token-value")
	require.NotContains(t, s, "Authorization")
}

func TestLogging_DoesNotLogCookie(t *testing.T) {
	t.Parallel()

	log := logmocks.NewMockLogger(t)
	var mu sync.Mutex
	var captured []any
	captureInfoArgs(log, "http request", &captured, &mu)

	handler := Logging(log)(okHandler())

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "sensitive-cookie-payload-abc"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	s := argsAsString(captured)
	require.NotContains(t, s, "sensitive-cookie-payload-abc")
	require.NotContains(t, s, "refresh_token")
	require.NotContains(t, s, "Cookie")
}
