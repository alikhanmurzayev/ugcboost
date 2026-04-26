package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestEncodeJSON(t *testing.T) {
	t.Parallel()

	t.Run("encoder failure logs without panic", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		// Channel values cannot be JSON-marshalled — Encode returns an error,
		// which encodeJSON must surface via log.Error and not propagate further.
		log.EXPECT().Error(mock.Anything, "failed to encode response", mock.Anything).Once()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/foo", nil)
		encodeJSON(w, r, make(chan int), log)
	})
}
