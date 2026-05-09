package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
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

// TestRespondError_ValidationSentinels locks the contract that every TMA-flow
// ValidationError sentinel reaches respondError as a typed *ValidationError
// and produces 422 + the granular code. A regression that wraps the sentinel
// without preserving errors.As (e.g. fmt.Errorf without %w) would silently
// fall through to the default branch and become 500 — this test catches that.
func TestRespondError_ValidationSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		code string
	}{
		{"ErrCampaignCreatorNotInvited", domain.ErrCampaignCreatorNotInvited, domain.CodeCampaignCreatorNotInvited},
		{"ErrCampaignCreatorAlreadyAgreed", domain.ErrCampaignCreatorAlreadyAgreed, domain.CodeCampaignCreatorAlreadyAgreed},
		{"ErrCampaignCreatorDeclinedNeedReinvite", domain.ErrCampaignCreatorDeclinedNeedReinvite, domain.CodeCampaignCreatorDeclinedNeedReinvite},
		{"ErrInvalidTmaURL", domain.ErrInvalidTmaURL, domain.CodeInvalidTmaURL},
		{"ErrTmaURLConflict", domain.ErrTmaURLConflict, domain.CodeTmaURLConflict},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+" → 422 + granular code", func(t *testing.T) {
			t.Parallel()
			log := logmocks.NewMockLogger(t)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/dummy", nil)

			respondError(w, r, tc.err, log)

			require.Equal(t, http.StatusUnprocessableEntity, w.Code)
			var resp api.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, tc.code, resp.Error.Code)
			require.NotEmpty(t, resp.Error.Message, "user-facing message must be present")
		})
	}
}

func TestRespondError_TMAForbidden(t *testing.T) {
	t.Parallel()
	log := logmocks.NewMockLogger(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/dummy", nil)

	respondError(w, r, domain.ErrTMAForbidden, log)

	require.Equal(t, http.StatusForbidden, w.Code)
	var resp api.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, domain.CodeTMAForbidden, resp.Error.Code)
}
