package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBodyLimit_Limit(t *testing.T) {
	t.Parallel()

	t.Run("under limit reads body in full", func(t *testing.T) {
		t.Parallel()
		handler := BodyLimit(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, "hello", string(body))
			w.WriteHeader(http.StatusOK)
		}))

		r := httptest.NewRequest("POST", "/", strings.NewReader("hello"))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("exact limit reads body without error", func(t *testing.T) {
		t.Parallel()
		const limit = 10
		handler := BodyLimit(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Len(t, body, limit)
			w.WriteHeader(http.StatusOK)
		}))

		r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", limit)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("over limit surfaces MaxBytesError to inner handler", func(t *testing.T) {
		t.Parallel()
		handler := BodyLimit(10)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			require.Error(t, err)
			var maxErr *http.MaxBytesError
			require.ErrorAs(t, err, &maxErr, "expected *http.MaxBytesError")
			require.Equal(t, int64(10), maxErr.Limit)
		}))

		r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 100)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		// Status code is the inner handler's responsibility — BodyLimit itself
		// never writes a status. We only verify the error is surfaced correctly.
	})
}
