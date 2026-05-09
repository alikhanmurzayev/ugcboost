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

const trustMeWebhookTestSecret = "tm-secret"

func TestTrustMeWebhookAuth(t *testing.T) {
	t.Parallel()

	t.Run("non-webhook path passes through", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		})

		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.True(t, called)
		require.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("missing authorization header returns 401 with empty body", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on auth failure")
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on wrong token")
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		r.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
	})

	t.Run("raw token without Bearer scheme rejected", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run without Bearer scheme")
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		r.Header.Set("Authorization", trustMeWebhookTestSecret)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("token with extra trailing bytes rejected", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on padded token")
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		r.Header.Set("Authorization", "Bearer "+trustMeWebhookTestSecret+"x")
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("empty configured secret denies even matching empty header (fail-closed)", func(t *testing.T) {
		t.Parallel()
		// subtle.ConstantTimeCompare("", "") == 1 — без явной guard'ы
		// middleware пропустит anonymous POST. Defense-in-depth поверх
		// config.Load() guard для staging/prod.
		log := logmocks.NewMockLogger(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler must not run with empty secret")
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth("", log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
	})

	t.Run("valid Bearer token passes to next", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		r.Header.Set("Authorization", "Bearer "+trustMeWebhookTestSecret)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.True(t, called)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("lowercase bearer scheme accepted (case-insensitive)", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		r.Header.Set("Authorization", "bearer "+trustMeWebhookTestSecret)
		w := httptest.NewRecorder()
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.True(t, called)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("encoder failure on 401 is logged, not panicked", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "trustme webhook 401 encode failed", mock.Anything).Once()

		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler should not run on auth failure")
		})

		w := &failingTrustMeWriter{header: http.Header{}}
		r := httptest.NewRequest(http.MethodPost, TrustMeWebhookPath, nil)
		TrustMeWebhookAuth(trustMeWebhookTestSecret, log)(next).ServeHTTP(w, r)

		require.Equal(t, http.StatusUnauthorized, w.status)
	})
}

type failingTrustMeWriter struct {
	header http.Header
	status int
}

func (f *failingTrustMeWriter) Header() http.Header { return f.header }
func (f *failingTrustMeWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced write failure")
}
func (f *failingTrustMeWriter) WriteHeader(code int) { f.status = code }
