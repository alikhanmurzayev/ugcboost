package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

const tmaTestValidToken = "abc_padding_secrettokenxx"

func newTmaServer(t *testing.T, authzSvc AuthzService, tmaSvc TmaCampaignCreatorService) *Server {
	t.Helper()
	return NewServer(nil, nil, authzSvc, nil, nil, nil, nil, nil, tmaSvc, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t))
}

func TestServer_TmaAgree(t *testing.T) {
	t.Parallel()

	t.Run("regex-reject malformed token → 404 without service call", func(t *testing.T) {
		t.Parallel()
		// authz / svc both nil mocks — they MUST NOT be invoked.
		router := newTestRouter(t, newTmaServer(t, mocks.NewMockAuthzService(t), mocks.NewMockTmaCampaignCreatorService(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/short/agree", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("happy agree returns 200 with already_decided=false", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
				CurrentStatus:     domain.CampaignCreatorStatusInvited,
			}, nil)

		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionAgree).
			Return(domain.CampaignCreatorDecisionResult{
				Status:         domain.CampaignCreatorStatusAgreed,
				AlreadyDecided: false,
			}, nil)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.TmaDecisionResult](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CampaignCreatorStatus(domain.CampaignCreatorStatusAgreed), resp.Status)
		require.False(t, resp.AlreadyDecided)
	})

	t.Run("authz forbidden → 403", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{}, domain.ErrTMAForbidden)

		router := newTestRouter(t, newTmaServer(t, authzSvc, mocks.NewMockTmaCampaignCreatorService(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeTMAForbidden, resp.Error.Code)
	})

	t.Run("authz campaign not found → 404", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{}, domain.ErrCampaignNotFound)

		router := newTestRouter(t, newTmaServer(t, authzSvc, mocks.NewMockTmaCampaignCreatorService(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service granular 422 propagated", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)
		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionAgree).
			Return(domain.CampaignCreatorDecisionResult{}, domain.ErrCampaignCreatorDeclinedNeedReinvite)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorDeclinedNeedReinvite, resp.Error.Code)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)
		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionAgree).
			Return(domain.CampaignCreatorDecisionResult{}, errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/tma/campaigns/"+tmaTestValidToken+"/agree")
		router := newTestRouter(t, NewServer(nil, nil, authzSvc, nil, nil, nil, nil, nil, tmaSvc, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("idempotent agree returns 200 with already_decided=true", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)

		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionAgree).
			Return(domain.CampaignCreatorDecisionResult{
				Status:         domain.CampaignCreatorStatusAgreed,
				AlreadyDecided: true,
			}, nil)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.TmaDecisionResult](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/agree", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CampaignCreatorStatus(domain.CampaignCreatorStatusAgreed), resp.Status)
		require.True(t, resp.AlreadyDecided)
	})
}

func TestServer_TmaDecline(t *testing.T) {
	t.Parallel()

	t.Run("regex-reject malformed token → 404", func(t *testing.T) {
		t.Parallel()
		router := newTestRouter(t, newTmaServer(t, mocks.NewMockAuthzService(t), mocks.NewMockTmaCampaignCreatorService(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/short/decline", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("happy decline returns 200 with already_decided=false", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)

		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionDecline).
			Return(domain.CampaignCreatorDecisionResult{
				Status:         domain.CampaignCreatorStatusDeclined,
				AlreadyDecided: false,
			}, nil)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.TmaDecisionResult](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/decline", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CampaignCreatorStatus(domain.CampaignCreatorStatusDeclined), resp.Status)
	})

	t.Run("authz error propagated", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{}, domain.ErrCampaignNotFound)

		router := newTestRouter(t, newTmaServer(t, authzSvc, mocks.NewMockTmaCampaignCreatorService(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/decline", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service error propagated", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)
		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionDecline).
			Return(domain.CampaignCreatorDecisionResult{}, domain.ErrCampaignCreatorAlreadyAgreed)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/decline", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorAlreadyAgreed, resp.Error.Code)
	})

	t.Run("idempotent decline returns 200 with already_decided=true", func(t *testing.T) {
		t.Parallel()
		authzSvc := mocks.NewMockAuthzService(t)
		authzSvc.EXPECT().AuthorizeTMACampaignDecision(mock.Anything, tmaTestValidToken).
			Return(authz.TMACampaignDecisionAuth{
				CreatorID:         "cr-1",
				CampaignID:        "camp-1",
				CampaignCreatorID: "cc-1",
			}, nil)

		tmaSvc := mocks.NewMockTmaCampaignCreatorService(t)
		tmaSvc.EXPECT().ApplyDecision(mock.Anything,
			service.TmaDecisionAuth{
				CampaignID:        "camp-1",
				CreatorID:         "cr-1",
				CampaignCreatorID: "cc-1",
			},
			domain.CampaignCreatorDecisionDecline).
			Return(domain.CampaignCreatorDecisionResult{
				Status:         domain.CampaignCreatorStatusDeclined,
				AlreadyDecided: true,
			}, nil)

		router := newTestRouter(t, newTmaServer(t, authzSvc, tmaSvc))
		w, resp := doJSON[api.TmaDecisionResult](t, router, http.MethodPost, "/tma/campaigns/"+tmaTestValidToken+"/decline", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CampaignCreatorStatus(domain.CampaignCreatorStatusDeclined), resp.Status)
		require.True(t, resp.AlreadyDecided)
	})
}
