package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

const (
	campaignUUID = "11111111-1111-1111-1111-111111111111"
	creatorAUUID = "22222222-2222-2222-2222-222222222222"
	creatorBUUID = "33333333-3333-3333-3333-333333333333"
)

var (
	campaignCreatorsPath = "/campaigns/" + campaignUUID + "/creators"
	campaignNotifyPath   = "/campaigns/" + campaignUUID + "/notify"
	campaignRemindPath   = "/campaigns/" + campaignUUID + "/remind-invitation"
)

func TestServer_AddCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("empty creatorIds → 422 CAMPAIGN_CREATOR_IDS_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsRequired, resp.Error.Code)
	})

	t.Run("over 200 creatorIds → 422 CAMPAIGN_CREATOR_IDS_TOO_MANY", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)

		// 201 unique uuids — one over the maxItems=200 cap.
		ids := make([]openapi_types.UUID, 201)
		for i := range ids {
			ids[i] = uuid.New()
		}

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: ids})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsTooMany, resp.Error.Code)
	})

	t.Run("duplicate creatorIds → 422 CAMPAIGN_CREATOR_IDS_DUPLICATES", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{
				uuid.MustParse(creatorAUUID), uuid.MustParse(creatorAUUID),
			}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsDuplicates, resp.Error.Code)
	})

	t.Run("service ErrCampaignNotFound → 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service ErrCampaignCreatorCreatorNotFound → 422 CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, domain.ErrCampaignCreatorCreatorNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorNotFound, resp.Error.Code)
	})

	t.Run("service ErrCreatorAlreadyInCampaign → 422 CREATOR_ALREADY_IN_CAMPAIGN", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, domain.ErrCreatorAlreadyInCampaign)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorAlreadyInCampaign, resp.Error.Code)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})

	t.Run("success returns 201 with items mapped to API shape", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID, creatorBUUID}).
			Return([]*domain.CampaignCreator{
				{ID: "44444444-4444-4444-4444-444444444444", CampaignID: campaignUUID, CreatorID: creatorAUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
				{ID: "55555555-5555-5555-5555-555555555555", CampaignID: campaignUUID, CreatorID: creatorBUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.AddCampaignCreatorsResult](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{
				uuid.MustParse(creatorAUUID), uuid.MustParse(creatorBUUID),
			}})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Len(t, resp.Data.Items, 2)
		require.Equal(t, api.CampaignCreator{
			Id:            uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			CampaignId:    uuid.MustParse(campaignUUID),
			CreatorId:     uuid.MustParse(creatorAUUID),
			Status:        api.Planned,
			InvitedAt:     nil,
			InvitedCount:  0,
			RemindedAt:    nil,
			RemindedCount: 0,
			DecidedAt:     nil,
			CreatedAt:     created,
			UpdatedAt:     created,
		}, resp.Data.Items[0])
		require.Equal(t, api.CampaignCreator{
			Id:            uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			CampaignId:    uuid.MustParse(campaignUUID),
			CreatorId:     uuid.MustParse(creatorBUUID),
			Status:        api.Planned,
			InvitedAt:     nil,
			InvitedCount:  0,
			RemindedAt:    nil,
			RemindedCount: 0,
			DecidedAt:     nil,
			CreatedAt:     created,
			UpdatedAt:     created,
		}, resp.Data.Items[1])
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return([]*domain.CampaignCreator{
				{ID: "not-a-uuid", CampaignID: campaignUUID, CreatorID: creatorAUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("corrupted CampaignID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return([]*domain.CampaignCreator{
				{ID: "44444444-4444-4444-4444-444444444444", CampaignID: "not-a-uuid", CreatorID: creatorAUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("corrupted CreatorID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanAddCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().Add(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return([]*domain.CampaignCreator{
				{ID: "44444444-4444-4444-4444-444444444444", CampaignID: campaignUUID, CreatorID: "not-a-uuid",
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignCreatorsPath,
			api.AddCampaignCreatorsInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestServer_RemoveCampaignCreator(t *testing.T) {
	t.Parallel()

	removePath := "/campaigns/" + campaignUUID + "/creators/" + creatorAUUID

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("service ErrCampaignNotFound → 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Remove(mock.Anything, campaignUUID, creatorAUUID).
			Return(domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service ErrCampaignCreatorNotFound → 404 CAMPAIGN_CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Remove(mock.Anything, campaignUUID, creatorAUUID).
			Return(domain.ErrCampaignCreatorNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorNotFound, resp.Error.Code)
	})

	t.Run("service ErrCampaignCreatorRemoveAfterAgreed → 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Remove(mock.Anything, campaignUUID, creatorAUUID).
			Return(domain.ErrCampaignCreatorRemoveAfterAgreed)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorRemoveAfterAgreed, resp.Error.Code)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Remove(mock.Anything, campaignUUID, creatorAUUID).
			Return(errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, removePath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success returns 204 with empty body", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemoveCampaignCreator(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Remove(mock.Anything, campaignUUID, creatorAUUID).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodDelete, removePath, nil)
		require.Equal(t, http.StatusNoContent, w.Code)
		require.Empty(t, w.Body.Bytes(), "204 must not carry a body")
	})
}

func TestServer_ListCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("service ErrCampaignNotFound → 404", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().List(mock.Anything, campaignUUID).Return(nil, domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().List(mock.Anything, campaignUUID).Return(nil, errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("empty list returns 200 with empty items", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().List(mock.Anything, campaignUUID).
			Return([]*domain.CampaignCreator{}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ListCampaignCreatorsResult](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, resp.Data.Items)
	})

	t.Run("success returns 200 with mapped items", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().List(mock.Anything, campaignUUID).
			Return([]*domain.CampaignCreator{
				{ID: "44444444-4444-4444-4444-444444444444", CampaignID: campaignUUID, CreatorID: creatorAUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ListCampaignCreatorsResult](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Items, 1)
		require.Equal(t, api.CampaignCreator{
			Id:            uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			CampaignId:    uuid.MustParse(campaignUUID),
			CreatorId:     uuid.MustParse(creatorAUUID),
			Status:        api.Planned,
			InvitedAt:     nil,
			InvitedCount:  0,
			RemindedAt:    nil,
			RemindedCount: 0,
			DecidedAt:     nil,
			CreatedAt:     created,
			UpdatedAt:     created,
		}, resp.Data.Items[0])
	})

	t.Run("corrupted ID from service surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
		ccSvc.EXPECT().List(mock.Anything, campaignUUID).
			Return([]*domain.CampaignCreator{
				{ID: "not-a-uuid", CampaignID: campaignUUID, CreatorID: creatorAUUID,
					Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created},
			}, nil)
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignCreatorsPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, campaignCreatorsPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestServer_NotifyCampaignCreators(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("empty creatorIds → 422 CAMPAIGN_CREATOR_IDS_REQUIRED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsRequired, resp.Error.Code)
	})

	t.Run("over 200 creatorIds → 422 CAMPAIGN_CREATOR_IDS_TOO_MANY", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)

		ids := make([]openapi_types.UUID, 201)
		for i := range ids {
			ids[i] = uuid.New()
		}
		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: ids})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsTooMany, resp.Error.Code)
	})

	t.Run("duplicate creatorIds → 422 CAMPAIGN_CREATOR_IDS_DUPLICATES", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{
				uuid.MustParse(creatorAUUID), uuid.MustParse(creatorAUUID),
			}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorIdsDuplicates, resp.Error.Code)
	})

	t.Run("service ErrCampaignNotFound → 404 CAMPAIGN_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Notify(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, domain.ErrCampaignNotFound)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCampaignNotFound, resp.Error.Code)
	})

	t.Run("service CampaignCreatorBatchInvalidError → 422 with custom schema details", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Notify(mock.Anything, campaignUUID, []string{creatorAUUID, creatorBUUID}).
			Return(nil, &domain.CampaignCreatorBatchInvalidError{
				Details: []domain.BatchValidationDetail{
					{CreatorID: creatorAUUID, Reason: domain.BatchInvalidReasonWrongStatus, CurrentStatus: domain.CampaignCreatorStatusAgreed},
					{CreatorID: creatorBUUID, Reason: domain.BatchInvalidReasonNotInCampaign},
				},
			})

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignCreatorBatchInvalidErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{
				uuid.MustParse(creatorAUUID), uuid.MustParse(creatorBUUID),
			}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorBatchInvalid, resp.Error.Code)
		require.Len(t, resp.Error.Details, 2)
		require.Equal(t, uuid.MustParse(creatorAUUID), uuid.UUID(resp.Error.Details[0].CreatorId))
		require.Equal(t, api.WrongStatus, resp.Error.Details[0].Reason)
		require.NotNil(t, resp.Error.Details[0].CurrentStatus)
		require.Equal(t, api.Agreed, *resp.Error.Details[0].CurrentStatus)
		require.Equal(t, uuid.MustParse(creatorBUUID), uuid.UUID(resp.Error.Details[1].CreatorId))
		require.Equal(t, api.NotInCampaign, resp.Error.Details[1].Reason)
		require.Nil(t, resp.Error.Details[1].CurrentStatus)
	})

	t.Run("happy path with empty undelivered → 200", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Notify(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignNotifyResult](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, resp.Data.Undelivered)
	})

	t.Run("partial-success surfaces undelivered list", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Notify(mock.Anything, campaignUUID, []string{creatorAUUID, creatorBUUID}).
			Return([]domain.NotifyFailure{
				{CreatorID: creatorAUUID, Reason: domain.NotifyFailureReasonBotBlocked},
			}, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignNotifyResult](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{
				uuid.MustParse(creatorAUUID), uuid.MustParse(creatorBUUID),
			}})
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Undelivered, 1)
		require.Equal(t, uuid.MustParse(creatorAUUID), uuid.UUID(resp.Data.Undelivered[0].CreatorId))
		require.Equal(t, api.BotBlocked, resp.Data.Undelivered[0].Reason)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanNotifyCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().Notify(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignNotifyPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignNotifyPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})
}

func TestServer_RemindCampaignCreatorsInvitation(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemindCampaignCreators(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignRemindPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("happy path with empty undelivered → 200", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemindCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().RemindInvitation(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignNotifyResult](t, router, http.MethodPost, campaignRemindPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, resp.Data.Undelivered)
	})

	t.Run("service batch-invalid → 422 with details", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemindCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().RemindInvitation(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, &domain.CampaignCreatorBatchInvalidError{
				Details: []domain.BatchValidationDetail{
					{CreatorID: creatorAUUID, Reason: domain.BatchInvalidReasonWrongStatus, CurrentStatus: domain.CampaignCreatorStatusPlanned},
				},
			})

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CampaignCreatorBatchInvalidErrorResponse](t, router, http.MethodPost, campaignRemindPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignCreatorBatchInvalid, resp.Error.Code)
		require.Len(t, resp.Error.Details, 1)
		require.Equal(t, api.WrongStatus, resp.Error.Details[0].Reason)
		require.NotNil(t, resp.Error.Details[0].CurrentStatus)
		require.Equal(t, api.Planned, *resp.Error.Details[0].CurrentStatus)
	})

	t.Run("service generic error → 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRemindCampaignCreators(mock.Anything).Return(nil)
		ccSvc := mocks.NewMockCampaignCreatorService(t)
		ccSvc.EXPECT().RemindInvitation(mock.Anything, campaignUUID, []string{creatorAUUID}).
			Return(nil, errors.New("db unavailable"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, campaignRemindPath)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, ccSvc, nil, nil, nil, ServerConfig{Version: "test-version"}, log))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, campaignRemindPath,
			api.CampaignCreatorBatchInput{CreatorIds: []openapi_types.UUID{uuid.MustParse(creatorAUUID)}})
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, resp.Error.Code)
	})
}
