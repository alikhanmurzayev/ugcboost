package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

// serverWithAuthzAndCreatorRead wires authz + creator (read-side service)
// while leaving the rest nil. Mirrors serverWithAuthzAndCreatorAndDict but for
// the GET /creators/{id} surface, which does not exercise application reads
// or the dictionary service (hydration happens inside CreatorService).
func serverWithAuthzAndCreatorRead(t *testing.T, authz AuthzService, creators CreatorService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, authz, nil, nil, creators, nil, ServerConfig{Version: "test-version"}, log)
}

func TestServer_GetCreator(t *testing.T) {
	t.Parallel()

	const creatorPath = "/creators/aaaa1111-1111-1111-1111-111111111111"
	creatorID := uuid.MustParse("aaaa1111-1111-1111-1111-111111111111")
	sourceAppID := uuid.MustParse("bbbb2222-2222-2222-2222-222222222222")
	verifierID := uuid.MustParse("cccc3333-3333-3333-3333-333333333333")

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreator(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorRead(t, authz, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, creatorPath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("not found returns 404 CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreator(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		creator.EXPECT().GetByID(mock.Anything, creatorID.String()).Return(nil, domain.ErrCreatorNotFound)

		router := newTestRouter(t, serverWithAuthzAndCreatorRead(t, authz, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, creatorPath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCreatorNotFound, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreator(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		creator.EXPECT().GetByID(mock.Anything, creatorID.String()).Return(nil, errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, creatorPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorRead(t, authz, creator, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, creatorPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success returns full aggregate", func(t *testing.T) {
		t.Parallel()
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Minute)
		verifiedAt := time.Date(2026, 5, 5, 11, 30, 0, 0, time.UTC)
		socialIGID := uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111")
		socialTTID := uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222")

		aggregate := &domain.CreatorAggregate{
			ID:                  creatorID.String(),
			IIN:                 "950515312348",
			SourceApplicationID: sourceAppID.String(),
			LastName:            "Муратова",
			FirstName:           "Айдана",
			MiddleName:          pointer.ToString("Ивановна"),
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			CityName:            "Алматы",
			Address:             pointer.ToString("ул. Абая 10"),
			CategoryOtherText:   pointer.ToString("стримы"),
			TelegramUserID:      424242,
			TelegramUsername:    pointer.ToString("aidana_tg"),
			TelegramFirstName:   pointer.ToString("Айдана"),
			TelegramLastName:    pointer.ToString("Муратова"),
			Socials: []domain.CreatorAggregateSocial{
				{
					ID:         socialIGID.String(),
					Platform:   domain.SocialPlatformInstagram,
					Handle:     "aidana",
					Verified:   true,
					Method:     pointer.ToString(domain.SocialVerificationMethodAuto),
					VerifiedAt: &verifiedAt,
					CreatedAt:  created,
				},
				{
					ID:               socialTTID.String(),
					Platform:         domain.SocialPlatformTikTok,
					Handle:           "aidana_tt",
					Verified:         true,
					Method:           pointer.ToString(domain.SocialVerificationMethodManual),
					VerifiedByUserID: pointer.ToString(verifierID.String()),
					VerifiedAt:       &verifiedAt,
					CreatedAt:        created,
				},
			},
			Categories: []domain.CreatorAggregateCategory{
				{Code: "beauty", Name: "Красота"},
				{Code: "fashion", Name: "Мода"},
			},
			CreatedAt: created,
			UpdatedAt: updated,
		}

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreator(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		creator.EXPECT().GetByID(mock.Anything, creatorID.String()).Return(aggregate, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorRead(t, authz, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorResult](t, router, http.MethodGet, creatorPath, nil)
		require.Equal(t, http.StatusOK, w.Code)

		methodAuto := api.SocialVerificationMethod(domain.SocialVerificationMethodAuto)
		methodManual := api.SocialVerificationMethod(domain.SocialVerificationMethodManual)
		expected := api.GetCreatorResult{
			Data: api.CreatorAggregate{
				Id:                  creatorID,
				Iin:                 "950515312348",
				SourceApplicationId: sourceAppID,
				LastName:            "Муратова",
				FirstName:           "Айдана",
				MiddleName:          pointer.ToString("Ивановна"),
				BirthDate:           openapi_types.Date{Time: birth},
				Phone:               "+77001234567",
				CityCode:            "almaty",
				CityName:            "Алматы",
				Address:             pointer.ToString("ул. Абая 10"),
				CategoryOtherText:   pointer.ToString("стримы"),
				TelegramUserId:      424242,
				TelegramUsername:    pointer.ToString("aidana_tg"),
				TelegramFirstName:   pointer.ToString("Айдана"),
				TelegramLastName:    pointer.ToString("Муратова"),
				Socials: []api.CreatorAggregateSocial{
					{
						Id:         socialIGID,
						Platform:   api.Instagram,
						Handle:     "aidana",
						Verified:   true,
						Method:     &methodAuto,
						VerifiedAt: &verifiedAt,
						CreatedAt:  created,
					},
					{
						Id:               socialTTID,
						Platform:         api.Tiktok,
						Handle:           "aidana_tt",
						Verified:         true,
						Method:           &methodManual,
						VerifiedByUserId: &verifierID,
						VerifiedAt:       &verifiedAt,
						CreatedAt:        created,
					},
				},
				Categories: []api.CreatorAggregateCategory{
					{Code: "beauty", Name: "Красота"},
					{Code: "fashion", Name: "Мода"},
				},
				CreatedAt: created,
				UpdatedAt: updated,
			},
		}
		require.Equal(t, expected, resp)
	})
}
