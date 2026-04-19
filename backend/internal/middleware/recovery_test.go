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

func TestRecovery_Panic(t *testing.T) {
	t.Parallel()

	t.Run("no panic", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		handler := Recovery(log)(okHandler())

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("panic returns 500", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "panic recovered", mock.MatchedBy(func(args []any) bool {
			if len(args) != 6 {
				return false
			}
			return args[0] == "panic" && args[2] == "stack" && args[4] == "path" && args[5] == "/"
		})).Once()

		handler := Recovery(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("something broke")
		}))

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))

		resp := parseError(t, w)
		require.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	})

	t.Run("panic with error", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "panic recovered", mock.MatchedBy(func(args []any) bool {
			return len(args) == 6 && args[0] == "panic" && args[2] == "stack" && args[4] == "path"
		})).Once()

		handler := Recovery(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic(errors.New("assert.AnError general error for testing"))
		}))

		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
