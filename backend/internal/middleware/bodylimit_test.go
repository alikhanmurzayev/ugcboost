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

	t.Run("under limit", func(t *testing.T) {
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

	t.Run("over limit", func(t *testing.T) {
		t.Parallel()
		handler := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			require.Error(t, err)
		}))

		r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 100)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
	})
}
