package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
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

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
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

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			map[string]any{"name": 123, "tmaUrl": "https://x"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("empty name after trim → 422 CAMPAIGN_NAME_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "   ", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameRequired, resp.Error.Code)
	})

	t.Run("name >255 → 422 CAMPAIGN_NAME_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: strings.Repeat("a", 256), TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameTooLong, resp.Error.Code)
	})

	t.Run("empty tmaUrl after trim → 422 CAMPAIGN_TMA_URL_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "   "})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignTmaURLRequired, resp.Error.Code)
	})

	t.Run("tmaUrl >2048 → 422 CAMPAIGN_TMA_URL_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
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
		campaigns.EXPECT().CreateCampaign(mock.Anything, domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc"}).
			Return((*domain.Campaign)(nil), domain.ErrCampaignNameTaken)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
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
		campaigns.EXPECT().CreateCampaign(mock.Anything, domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc"}).
			Return((*domain.Campaign)(nil), errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("success trims whitespace and returns 201 with id-only payload", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().CreateCampaign(mock.Anything, domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc"}).
			Return(&domain.Campaign{
				ID:     "11111111-2222-3333-4444-555555555555",
				Name:   "Promo X",
				TmaURL: "https://tma.ugcboost.kz/tz/abc",
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignCreatedResult](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "  Promo X  ", TmaUrl: "  https://tma.ugcboost.kz/tz/abc  "})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.CampaignCreatedResult{
			Data: api.CampaignCreatedData{
				Id: uuid.MustParse("11111111-2222-3333-4444-555555555555"),
			},
		}, resp)
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanCreateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().CreateCampaign(mock.Anything, domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc"}).
			Return(&domain.Campaign{
				ID:     "not-a-uuid",
				Name:   "Promo X",
				TmaURL: "https://tma.ugcboost.kz/tz/abc",
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/campaigns",
			api.CampaignInput{Name: "Promo X", TmaUrl: "https://tma.ugcboost.kz/tz/abc"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestServer_GetCampaign(t *testing.T) {
	t.Parallel()

	const campaignPath = "/campaigns/aaaa1111-1111-1111-1111-111111111111"
	campaignID := uuid.MustParse("aaaa1111-1111-1111-1111-111111111111")

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCampaign(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignPath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("not found returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().GetByID(mock.Anything, campaignID.String()).
			Return((*domain.Campaign)(nil), domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignPath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
		require.Equal(t, "Кампания не найдена.", resp.Error.Message)
	})

	t.Run("service generic error returns 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().GetByID(mock.Anything, campaignID.String()).
			Return((*domain.Campaign)(nil), errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().GetByID(mock.Anything, campaignID.String()).
			Return(&domain.Campaign{
				ID:     "not-a-uuid",
				Name:   "Promo X",
				TmaURL: "https://tma.ugcboost.kz/tz/abc",
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success returns full campaign", func(t *testing.T) {
		t.Parallel()
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Minute)
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().GetByID(mock.Anything, campaignID.String()).
			Return(&domain.Campaign{
				ID:        campaignID.String(),
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: updated,
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCampaignResult](t, router, http.MethodGet, campaignPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.GetCampaignResult{
			Data: api.Campaign{
				Id:        campaignID,
				Name:      "Promo X",
				TmaUrl:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: updated,
			},
		}, resp)
	})
}

func TestServer_UpdateCampaign(t *testing.T) {
	t.Parallel()

	const campaignPath = "/campaigns/aaaa1111-1111-1111-1111-111111111111"
	const campaignIDStr = "aaaa1111-1111-1111-1111-111111111111"

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("empty name after trim → 422 CAMPAIGN_NAME_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "   ", TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameRequired, resp.Error.Code)
	})

	t.Run("name >255 → 422 CAMPAIGN_NAME_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: strings.Repeat("a", 256), TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNameTooLong, resp.Error.Code)
	})

	t.Run("empty tmaUrl after trim → 422 CAMPAIGN_TMA_URL_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: "   "})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignTmaURLRequired, resp.Error.Code)
	})

	t.Run("tmaUrl >2048 → 422 CAMPAIGN_TMA_URL_TOO_LONG", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: strings.Repeat("a", 2049)})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignTmaURLTooLong, resp.Error.Code)
	})

	t.Run("not found returns 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().UpdateCampaign(mock.Anything, campaignIDStr,
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new"}).
			Return(domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
		require.Equal(t, "Кампания не найдена.", resp.Error.Message)
	})

	t.Run("name taken → 409 CAMPAIGN_NAME_TAKEN", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().UpdateCampaign(mock.Anything, campaignIDStr,
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new"}).
			Return(domain.ErrCampaignNameTaken)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusConflict, w.Code)
		require.Equal(t, domain.CodeCampaignNameTaken, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "Кампания с таким названием")
	})

	t.Run("generic service error → 500 INTERNAL_ERROR", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().UpdateCampaign(mock.Anything, campaignIDStr,
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new"}).
			Return(errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "Promo Y", TmaUrl: "https://tma.ugcboost.kz/tz/new"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("success trims whitespace and returns 204 with empty body", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanUpdateCampaign(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().UpdateCampaign(mock.Anything, campaignIDStr,
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new"}).
			Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, _ := doJSON[struct{}](t, router, http.MethodPatch, campaignPath,
			api.CampaignInput{Name: "  Promo Y  ", TmaUrl: "  https://tma.ugcboost.kz/tz/new  "})
		require.Equal(t, http.StatusNoContent, w.Code)
		require.Zero(t, w.Body.Len(), "204 must have empty body")
	})
}

func TestServer_ListCampaigns(t *testing.T) {
	t.Parallel()

	const validQuery = "/campaigns?page=1&perPage=10&sort=created_at&order=desc"

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, validQuery, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("missing required page → 400 from wrapper", func(t *testing.T) {
		t.Parallel()
		// Required-param errors are converted by HandleParamError into 400 +
		// CodeValidation before the handler ever runs, so no authz/service mocks
		// participate. The 400 (not 422) matches the rest of the handler-layer
		// contract for malformed query params — see HandleParamError.
		router := newTestRouter(t, NewServer(nil, nil, mocks.NewMockAuthzService(t), nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/campaigns?perPage=10&sort=created_at&order=desc", nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("invalid sort enum → 422 CodeValidation", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/campaigns?page=1&perPage=10&sort=bogus&order=asc", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "sort")
	})

	t.Run("invalid order enum → 422 CodeValidation", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/campaigns?page=1&perPage=10&sort=created_at&order=sideways", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "order")
	})

	t.Run("page=0 → 422 CodeValidation", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/campaigns?page=0&perPage=10&sort=created_at&order=asc", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "page")
	})

	t.Run("perPage above maximum → 422 CodeValidation", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/campaigns?page=1&perPage=201&sort=created_at&order=asc", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "perPage")
	})

	t.Run("search above maxLength → 422 CodeValidation", func(t *testing.T) {
		t.Parallel()
		// oapi-codegen wrapper does NOT enforce maxLength at runtime — the
		// explicit handler check is the actual guard, so it must surface 422
		// with a search-specific message rather than letting a megabyte ILIKE
		// pattern reach Postgres.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		long := strings.Repeat("a", domain.CampaignListSearchMaxLen+1)
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet,
			"/campaigns?page=1&perPage=10&sort=created_at&order=asc&search="+long, nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "search")
	})

	t.Run("service error → 500 CodeInternal", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Return((*domain.CampaignListPage)(nil), errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, validQuery, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("empty page returns 200 with empty items array", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, domain.CampaignListInput{
			Search:    "",
			IsDeleted: nil,
			Sort:      domain.CampaignSortCreatedAt,
			Order:     domain.SortOrderDesc,
			Page:      5,
			PerPage:   10,
		}).Return(&domain.CampaignListPage{
			Items:   nil,
			Total:   0,
			Page:    5,
			PerPage: 10,
		}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignsListResult](t, router, http.MethodGet, "/campaigns?page=5&perPage=10&sort=created_at&order=desc", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, resp.Data.Items, "items must be a non-nil empty array, never JSON null")
		require.Empty(t, resp.Data.Items)
		require.EqualValues(t, 0, resp.Data.Total)
		require.Equal(t, 5, resp.Data.Page)
		require.Equal(t, 10, resp.Data.PerPage)
	})

	t.Run("beyond-last page returns empty items but non-zero total", func(t *testing.T) {
		t.Parallel()
		// I/O Matrix row "Page beyond last → 200 + items:[], total>0". Empty
		// list short-circuit returns total=0; this case proves the handler
		// surfaces total even when the requested page sits past the last row.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Return(&domain.CampaignListPage{
				Items:   nil,
				Total:   25,
				Page:    99,
				PerPage: 10,
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignsListResult](t, router, http.MethodGet, "/campaigns?page=99&perPage=10&sort=created_at&order=desc", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, resp.Data.Items)
		require.EqualValues(t, 25, resp.Data.Total)
		require.Equal(t, 99, resp.Data.Page)
	})

	t.Run("isDeleted=missing leaves nil at service boundary", func(t *testing.T) {
		t.Parallel()
		// Captured-input asserts that omitting isDeleted in the URL leaves
		// in.IsDeleted == nil at the service boundary — drift between query
		// parsing and domain.CampaignListInput would slip through otherwise.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CampaignListInput) {
				require.Nil(t, in.IsDeleted, "missing query param must remain nil")
			}).
			Return(&domain.CampaignListPage{Items: nil, Total: 0, Page: 1, PerPage: 10}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CampaignsListResult](t, router, http.MethodGet, "/campaigns?page=1&perPage=10&sort=created_at&order=asc", nil)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("happy path passes captured input and maps rows", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)

		isDeleted := false
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)
		// Captured input asserts that the handler passes the trimmed search,
		// the validated enums and the parsed isDeleted pointer to the service
		// — drift between query-param parsing and domain.CampaignListInput is
		// exactly what this assertion guards against.
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CampaignListInput) {
				require.Equal(t, domain.CampaignListInput{
					Search:    "promo",
					IsDeleted: pointer.ToBool(false),
					Sort:      domain.CampaignSortName,
					Order:     domain.SortOrderAsc,
					Page:      2,
					PerPage:   25,
				}, in)
			}).
			Return(&domain.CampaignListPage{
				Items: []*domain.Campaign{
					{ID: "11111111-2222-3333-4444-555555555555", Name: "Promo A", TmaURL: "https://tma.ugcboost.kz/tz/a", IsDeleted: false, CreatedAt: created, UpdatedAt: updated},
					{ID: "22222222-3333-4444-5555-666666666666", Name: "Promo B", TmaURL: "https://tma.ugcboost.kz/tz/b", IsDeleted: false, CreatedAt: created, UpdatedAt: updated},
				},
				Total:   42,
				Page:    2,
				PerPage: 25,
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		// Search has surrounding whitespace so we exercise the trim contract.
		w, resp := doJSON[api.CampaignsListResult](t, router, http.MethodGet,
			"/campaigns?page=2&perPage=25&sort=name&order=asc&search=%20promo%20&isDeleted=false", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.EqualValues(t, 42, resp.Data.Total)
		require.Equal(t, 2, resp.Data.Page)
		require.Equal(t, 25, resp.Data.PerPage)
		require.Len(t, resp.Data.Items, 2)
		require.Equal(t, "Promo A", resp.Data.Items[0].Name)
		require.Equal(t, "Promo B", resp.Data.Items[1].Name)
		require.Equal(t, uuid.MustParse("11111111-2222-3333-4444-555555555555"), resp.Data.Items[0].Id)
		require.False(t, resp.Data.Items[0].IsDeleted)
		require.Equal(t, isDeleted, resp.Data.Items[0].IsDeleted)
	})

	t.Run("isDeleted=true filter is forwarded to service", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CampaignListInput) {
				require.NotNil(t, in.IsDeleted)
				require.True(t, *in.IsDeleted)
			}).
			Return(&domain.CampaignListPage{Items: nil, Total: 0, Page: 1, PerPage: 10}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CampaignsListResult](t, router, http.MethodGet,
			"/campaigns?page=1&perPage=10&sort=created_at&order=asc&isDeleted=true", nil)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaigns(mock.Anything).Return(nil)
		campaigns := mocks.NewMockCampaignService(t)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Return(&domain.CampaignListPage{
				Items: []*domain.Campaign{
					{ID: "not-a-uuid", Name: "Broken"},
				},
				Total:   1,
				Page:    1,
				PerPage: 10,
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/campaigns")

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, campaigns, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, validQuery, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code,
			"corrupted UUID surfaces through respondError default branch as 500/INTERNAL")
	})
}
