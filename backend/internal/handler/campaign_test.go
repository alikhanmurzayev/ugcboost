package handler

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestServer_CreateCampaign(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("invalid JSON shape", func(t *testing.T) {
		t.Parallel()
		// strict-server decodes the body before the handler runs, so authz is
		// never called when JSON is malformed — the request rejects at the
		// adapter layer with 422+CodeValidation via RequestErrorHandlerFunc.
		authz := mocks.NewMockAuthzService(t)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			map[string]any{"name": 123, "tmaUrl": "https://x"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("empty name after trim → 422 CAMPAIGN_NAME_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "   ", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameRequired, resp.Error.Code)
	})

	t.Run("name >255 → 422 CAMPAIGN_NAME_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: strings.Repeat("a", 256), TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameTooLong, resp.Error.Code)
	})

	t.Run("empty tmaUrl after trim → 422 CAMPAIGN_TMA_URL_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "   "})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignTmaURLRequired, resp.Error.Code)
	})

	t.Run("tmaUrl >2048 → 422 CAMPAIGN_TMA_URL_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: strings.Repeat("a", 2049)})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignTmaURLTooLong, resp.Error.Code)
	})

	t.Run("name taken (race) → 409 CAMPAIGN_NAME_TAKEN", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().CreateCampaign(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return((*domain.Campaign)(nil), domain.ErrCampaignNameTaken)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusConflict, w.Code)
		require.Equal(t, domain.CodeCampaignNameTaken, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "Кампания с таким названием")
	})

	t.Run("generic service error → 500 INTERNAL_ERROR", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().CreateCampaign(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return((*domain.Campaign)(nil), errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("success trims whitespace and returns 201", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		campaigns.EXPECT().CreateCampaign(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return(&domain.Campaign{
				ID:        "11111111-2222-3333-4444-555555555555",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignResult](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "  Promo X  ", TmaUrl: "  https://tma.ugcboost.kz/tz/abc  "})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.CampaignResult{
			Data: api.Campaign{
				Id:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
				Name:      "Promo X",
				TmaUrl:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			},
		}, resp)
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		campaigns.EXPECT().CreateCampaign(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return(&domain.Campaign{
				ID:        "not-a-uuid",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
