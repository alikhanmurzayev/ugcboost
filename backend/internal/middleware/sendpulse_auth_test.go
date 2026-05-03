package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

const sendPulseTestSecret = "s3cret"

func TestSendPulseAuth(t *testing.T) {
	t.Parallel()

	t.Run("non-webhook path passes through unchanged", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		})

		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.True(t, called)
		require.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("missing authorization header returns 401 with empty body", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on auth failure")
		})

		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("wrong scheme returns 401", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on auth failure")
		})

		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		r.Header.Set("Authorization", "Basic "+sendPulseTestSecret)
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong secret returns 401", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on wrong secret")
		})

		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		r.Header.Set("Authorization", "Bearer wrong-secret")
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("valid bearer secret passes to next handler", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		r.Header.Set("Authorization", "Bearer "+sendPulseTestSecret)
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.True(t, called)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("case-insensitive scheme rejected to keep parser strict", func(t *testing.T) {
		t.Parallel()
		// Spec freezes the bearer prefix as exactly "Bearer "; lower-case
		// "bearer " must fail. Confirms we do not let SendPulse rewrite the
		// header casing without re-issuing a secret rotation.
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on lower-case scheme")
		})

		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		r.Header.Set("Authorization", "bearer "+sendPulseTestSecret)
		w := httptest.NewRecorder()
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("encoder failure on 401 is logged, not panicked", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "sendpulse webhook 401 encode failed", mock.Anything).Once()

		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on auth failure")
		})

		// failingWriter pretends to be an http.ResponseWriter but errors on
		// every Write — exercising the err branch in writeSendPulseUnauthorized.
		w := &failingResponseWriter{header: http.Header{}}
		r := httptest.NewRequest(http.MethodPost, SendPulseWebhookPath, nil)
		SendPulseAuth(sendPulseTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.status)
	})
}

// failingResponseWriter satisfies http.ResponseWriter and always returns an
// error on Write, so json.NewEncoder.Encode surfaces a non-nil error.
type failingResponseWriter struct {
	header http.Header
	status int
}

func (f *failingResponseWriter) Header() http.Header { return f.header }
func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced write failure")
}
func (f *failingResponseWriter) WriteHeader(code int) { f.status = code }
