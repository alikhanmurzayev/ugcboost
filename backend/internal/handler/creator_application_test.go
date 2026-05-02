package handler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// validRequest returns a payload that the handler will accept end-to-end so
// scenarios can mutate one field to hit a specific branch. Address is set as
// a pointer so the "every request field forwards verbatim" success test can
// assert non-nil pass-through; scenarios that need the field absent override
// it with nil before sending.
func validRequest() api.CreatorApplicationSubmitRequest {
	return api.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: pointer.ToString("Ивановна"),
		Iin:        "950515312348",
		Phone:      "+77001234567",
		City:       "almaty",
		Address:    pointer.ToString("ул. Абая 1"),
		Categories: []string{"beauty", "fashion"},
		Socials: []api.SocialAccountInput{
			{Platform: api.Instagram, Handle: "@aidana"},
			{Platform: api.Tiktok, Handle: "aidana_tt"},
		},
		AcceptedAll: true,
	}
}

func serverWithCreator(t *testing.T, creator CreatorApplicationService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, nil, nil, creator, nil, ServerConfig{
		Version:               "test-version",
		TelegramBotUsername:   "ugcboost_test_bot",
		LegalAgreementVersion: "2026-04-20",
		LegalPrivacyVersion:   "2026-04-20",
	}, log)
}

func TestServer_SubmitCreatorApplication(t *testing.T) {
	t.Parallel()

	t.Run("invalid json body returns 422", func(t *testing.T) {
		t.Parallel()
		router := newTestRouter(t, serverWithCreator(t, nil, logmocks.NewMockLogger(t)))

		req := httptest.NewRequest(http.MethodPost, "/creators/applications", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("service validation error passes through as 422", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(nil, domain.NewValidationError(domain.CodeInvalidIIN, "Некорректный ИИН"))

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeInvalidIIN, resp.Error.Code)
	})

	t.Run("duplicate returns 409", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(nil, domain.NewBusinessError(domain.CodeCreatorApplicationDuplicate, "already exists"))

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusConflict, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationDuplicate, resp.Error.Code)
	})

	t.Run("success returns 201 with deep link and forwards every request field", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationInput
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationInput) {
				captured = in
			}).
			Return(&domain.CreatorApplicationSubmission{
				ApplicationID: appID.String(),
				BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			}, nil)

		router := newTestRouterWithClientIP(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		before := time.Now().UTC()
		w, resp := doJSON[api.CreatorApplicationSubmitResult](t, router, http.MethodPost, "/creators/applications", validRequest(),
			func(r *http.Request) {
				r.Header.Set("User-Agent", "go-test/1")
				r.RemoteAddr = "203.0.113.7:4567"
			})
		after := time.Now().UTC()

		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.CreatorApplicationSubmitResult{
			Data: api.CreatorApplicationSubmitData{
				ApplicationId:  appID,
				TelegramBotUrl: "https://t.me/ugcboost_test_bot?start=" + appID.String(),
			},
		}, resp)

		// Every field forwarded verbatim, including IP from the ClientIP
		// middleware. Check Now separately (it's generated by the handler).
		require.WithinDuration(t, before, captured.Now, after.Sub(before)+time.Second)
		captured.Now = time.Time{}
		require.Equal(t, domain.CreatorApplicationInput{
			LastName:      "Муратова",
			FirstName:     "Айдана",
			MiddleName:    pointer.ToString("Ивановна"),
			IIN:           "950515312348",
			Phone:         "+77001234567",
			CityCode:      "almaty",
			Address:       pointer.ToString("ул. Абая 1"),
			CategoryCodes: []string{"beauty", "fashion"},
			Socials: []domain.SocialAccountInput{
				{Platform: "instagram", Handle: "@aidana"},
				{Platform: "tiktok", Handle: "aidana_tt"},
			},
			Consents:         domain.ConsentsInput{AcceptedAll: true},
			IPAddress:        "203.0.113.7",
			UserAgent:        "go-test/1",
			AgreementVersion: "2026-04-20",
			PrivacyVersion:   "2026-04-20",
		}, captured)
	})

	t.Run("address absent — handler forwards nil pointer to service", func(t *testing.T) {
		t.Parallel()
		// Landing form does not collect a legal address. Verify the handler
		// passes the optional pointer through unchanged so the service sees
		// nil and the row column stays NULL.
		appID := uuid.MustParse("44444444-5555-6666-7777-888888888888")
		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationInput
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationInput) {
				captured = in
			}).
			Return(&domain.CreatorApplicationSubmission{
				ApplicationID: appID.String(),
				BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			}, nil)

		req := validRequest()
		req.Address = nil

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CreatorApplicationSubmitResult](t, router, http.MethodPost, "/creators/applications", req)

		require.Equal(t, http.StatusCreated, w.Code)
		require.Nil(t, captured.Address)
	})

	t.Run("oversized user agent is truncated", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("22222222-3333-4444-5555-666666666666")
		oversized := strings.Repeat("x", middleware.MaxUserAgentLength+128)

		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationInput
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationInput) {
				captured = in
			}).
			Return(&domain.CreatorApplicationSubmission{
				ApplicationID: appID.String(),
				BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			}, nil)

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CreatorApplicationSubmitResult](t, router, http.MethodPost, "/creators/applications", validRequest(),
			func(r *http.Request) { r.Header.Set("User-Agent", oversized) })

		require.Equal(t, http.StatusCreated, w.Code)
		require.Len(t, captured.UserAgent, middleware.MaxUserAgentLength)
	})

	t.Run("service returns non-uuid application id yields 500", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(&domain.CreatorApplicationSubmission{
				ApplicationID: "not-a-uuid",
				BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			}, nil)

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/creators/applications")
		router := newTestRouter(t, serverWithCreator(t, creator, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("bot username with url-unsafe characters is escaped in deep link", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("33333333-4444-5555-6666-777777777777")
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(&domain.CreatorApplicationSubmission{
				ApplicationID: appID.String(),
				BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
			}, nil)

		// Simulate a misconfigured env: slashes and spaces should be escaped
		// rather than corrupting the URL path.
		server := NewServer(nil, nil, nil, nil, creator, nil, ServerConfig{
			Version:               "test-version",
			TelegramBotUsername:   "bad name/bot",
			LegalAgreementVersion: "2026-04-20",
			LegalPrivacyVersion:   "2026-04-20",
		}, logmocks.NewMockLogger(t))
		router := newTestRouter(t, server)
		w, resp := doJSON[api.CreatorApplicationSubmitResult](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, "https://t.me/bad%20name%2Fbot?start="+appID.String(), resp.Data.TelegramBotUrl)
	})
}

// newTestRouterWithClientIP wires the ClientIP and RequestMeta middleware the
// same way cmd/api does, so handler tests can verify that IPAddress and
// User-Agent flow from r.RemoteAddr / r.Header into the service input.
func newTestRouterWithClientIP(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	r.Use(middleware.ClientIP)
	r.Use(middleware.RequestMeta)
	api.HandlerWithOptions(NewStrictAPIHandler(s), api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
	})
	return r
}

func serverWithAuthzAndCreatorAndDict(t *testing.T, authz AuthzService, creator CreatorApplicationService, dict DictionaryService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, authz, nil, creator, dict, ServerConfig{
		Version: "test-version",
	}, log)
}

func TestServer_GetCreatorApplication(t *testing.T) {
	t.Parallel()

	const appPath = "/creators/applications/11111111-2222-3333-4444-555555555555"

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("not found maps sql.ErrNoRows to 404", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(nil, sql.ErrNoRows)

		// Dictionary service is not consulted on the not-found path: respondError
		// short-circuits before the mapper runs, so dict stays nil.
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(nil, errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, appPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("dictionary categories error returns 500", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{ID: appID.String()}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return(nil, errors.New("dict down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, appPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("dictionary cities error returns 500", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{ID: appID.String()}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return(nil, errors.New("dict down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, appPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success returns full aggregate", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
		acceptedAt := time.Date(2026, 4, 20, 18, 0, 1, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{
				ID:                appID.String(),
				LastName:          "Муратова",
				FirstName:         "Айдана",
				MiddleName:        pointer.ToString("Ивановна"),
				IIN:               "950515312348",
				BirthDate:         birth,
				Phone:             "+77001234567",
				CityCode:          "almaty",
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            domain.CreatorApplicationStatusVerification,
				CreatedAt:         created,
				UpdatedAt:         updated,
				// Domain returns codes in arbitrary order — the handler sorts
				// by (sortOrder, code) after dictionary resolution. Pass them
				// reversed to lock that contract.
				Categories: []string{"fashion", "beauty"},
				Socials: []domain.CreatorApplicationDetailSocial{
					{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
					{Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
				},
				Consents: []domain.CreatorApplicationDetailConsent{
					{ConsentType: domain.ConsentTypeProcessing, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: domain.ConsentTypeThirdParty, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: domain.ConsentTypeCrossBorder, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: domain.ConsentTypeTerms, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IPAddress: "127.0.0.1", UserAgent: "ua/1"},
				},
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return([]domain.DictionaryEntry{
				{Code: "beauty", Name: "Красота", SortOrder: 10},
				{Code: "fashion", Name: "Мода", SortOrder: 20},
				{Code: "food", Name: "Еда", SortOrder: 30},
			}, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{
				{Code: "almaty", Name: "Алматы", SortOrder: 100},
				{Code: "astana", Name: "Астана", SortOrder: 200},
			}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorApplicationResult](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.GetCreatorApplicationResult{
			Data: api.CreatorApplicationDetailData{
				Id:                appID,
				LastName:          "Муратова",
				FirstName:         "Айдана",
				MiddleName:        pointer.ToString("Ивановна"),
				Iin:               "950515312348",
				BirthDate:         openapi_types.Date{Time: birth},
				Phone:             "+77001234567",
				City:              api.CreatorApplicationDetailCity{Code: "almaty", Name: "Алматы", SortOrder: 100},
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            api.CreatorApplicationDetailDataStatusVerification,
				CreatedAt:         created,
				UpdatedAt:         updated,
				Categories: []api.CreatorApplicationDetailCategory{
					{Code: "beauty", Name: "Красота", SortOrder: 10},
					{Code: "fashion", Name: "Мода", SortOrder: 20},
				},
				Socials: []api.CreatorApplicationDetailSocial{
					{Platform: api.Instagram, Handle: "aidana"},
					{Platform: api.Tiktok, Handle: "aidana_tt"},
				},
				Consents: []api.CreatorApplicationDetailConsent{
					{ConsentType: api.Processing, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.ThirdParty, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.CrossBorder, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.Terms, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
				},
			},
		}, resp)
	})

	t.Run("deactivated category and city fall back to code", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{
				ID:         appID.String(),
				LastName:   "Муратова",
				FirstName:  "Айдана",
				IIN:        "950515312348",
				BirthDate:  birth,
				Phone:      "+77001234567",
				CityCode:   "atyrau", // not present in the active dictionary below
				Address:    pointer.ToString("ул. Абая 1"),
				Status:     domain.CreatorApplicationStatusVerification,
				CreatedAt:  created,
				UpdatedAt:  created,
				Categories: []string{"beauty", "vintage_asmr"}, // vintage_asmr deactivated
				Socials:    nil,
				Consents:   nil,
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return([]domain.DictionaryEntry{
				{Code: "beauty", Name: "Красота", SortOrder: 10},
			}, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{
				{Code: "almaty", Name: "Алматы", SortOrder: 100},
			}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorApplicationResult](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		// Deactivated categories surface as {code, name: code, sortOrder: 0};
		// the active "beauty" still resolves correctly. After the in-memory
		// sort by (sortOrder, code), the deactivated entry (sortOrder 0) wins
		// over "beauty" (sortOrder 10) — vintage_asmr comes first.
		require.Equal(t, []api.CreatorApplicationDetailCategory{
			{Code: "vintage_asmr", Name: "vintage_asmr", SortOrder: 0},
			{Code: "beauty", Name: "Красота", SortOrder: 10},
		}, resp.Data.Categories)
		require.Equal(t, api.CreatorApplicationDetailCity{
			Code: "atyrau", Name: "atyrau", SortOrder: 0,
		}, resp.Data.City)
	})
}

func TestMapCreatorApplicationStatusToAPI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		domainValue string
		apiValue    api.CreatorApplicationDetailDataStatus
	}{
		{domain.CreatorApplicationStatusVerification, api.CreatorApplicationDetailDataStatusVerification},
		{domain.CreatorApplicationStatusModeration, api.CreatorApplicationDetailDataStatusModeration},
		{domain.CreatorApplicationStatusAwaitingContract, api.CreatorApplicationDetailDataStatusAwaitingContract},
		{domain.CreatorApplicationStatusContractSent, api.CreatorApplicationDetailDataStatusContractSent},
		{domain.CreatorApplicationStatusSigned, api.CreatorApplicationDetailDataStatusSigned},
		{domain.CreatorApplicationStatusRejected, api.CreatorApplicationDetailDataStatusRejected},
		{domain.CreatorApplicationStatusWithdrawn, api.CreatorApplicationDetailDataStatusWithdrawn},
	}
	for _, tc := range cases {
		t.Run(tc.domainValue, func(t *testing.T) {
			t.Parallel()
			got, err := mapCreatorApplicationStatusToAPI(tc.domainValue)
			require.NoError(t, err)
			require.Equal(t, tc.apiValue, got)
		})
	}

	t.Run("unknown status returns error", func(t *testing.T) {
		t.Parallel()
		_, err := mapCreatorApplicationStatusToAPI("ghost")
		require.ErrorContains(t, err, "ghost")
	})
}

func TestMapCreatorApplicationStatusToListItemAPI(t *testing.T) {
	t.Parallel()

	t.Run("known status casts to list-item enum", func(t *testing.T) {
		t.Parallel()
		got, err := mapCreatorApplicationStatusToListItemAPI(domain.CreatorApplicationStatusVerification)
		require.NoError(t, err)
		require.Equal(t, api.CreatorApplicationListItemStatusVerification, got)
	})

	t.Run("unknown status surfaces error", func(t *testing.T) {
		t.Parallel()
		_, err := mapCreatorApplicationStatusToListItemAPI("ghost")
		require.ErrorContains(t, err, "ghost")
	})
}

func TestApiListStatusesToDomain(t *testing.T) {
	t.Parallel()

	t.Run("nil pointer returns nil", func(t *testing.T) {
		t.Parallel()
		out, err := apiListStatusesToDomain(nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		empty := []api.CreatorApplicationsListRequestStatuses{}
		out, err := apiListStatusesToDomain(&empty)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("known statuses dedup preserved order", func(t *testing.T) {
		t.Parallel()
		in := []api.CreatorApplicationsListRequestStatuses{
			api.Verification, api.Moderation, api.Verification,
		}
		out, err := apiListStatusesToDomain(&in)
		require.NoError(t, err)
		require.Equal(t, []string{"verification", "moderation"}, out)
	})

	t.Run("unknown status surfaces 422", func(t *testing.T) {
		t.Parallel()
		bad := []api.CreatorApplicationsListRequestStatuses{api.Verification, "ghost"}
		_, err := apiListStatusesToDomain(&bad)
		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeValidation, ve.Code)
		require.Contains(t, ve.Message, "ghost")
	})
}

func TestServer_ListCreatorApplications(t *testing.T) {
	t.Parallel()

	const listPath = "/creators/applications/list"

	validBody := func() api.CreatorApplicationsListRequest {
		return api.CreatorApplicationsListRequest{
			Sort:    api.CreatedAt,
			Order:   api.Desc,
			Page:    1,
			PerPage: 20,
		}
	}

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("invalid body returns 422 (strict-server decode error)", func(t *testing.T) {
		t.Parallel()
		// strict-server intercepts JSON decode failures BEFORE the handler runs,
		// so authz never sees this request — the wrapper short-circuits to 422.
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, nil, nil, nil, logmocks.NewMockLogger(t)))
		req := httptest.NewRequest(http.MethodPost, listPath, bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid sort returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Sort = "rating"
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "sort")
	})

	t.Run("invalid order returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Order = "random"
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "order")
	})

	t.Run("page below 1 returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Page = 0
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "page")
	})

	t.Run("perPage below 1 returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.PerPage = 0
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "perPage")
	})

	t.Run("perPage above 200 returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.PerPage = 201
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("ageFrom > ageTo returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.AgeFrom = pointer.ToInt(40)
		body.AgeTo = pointer.ToInt(20)
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "ageFrom")
	})

	t.Run("dateFrom after dateTo returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		body.DateFrom = &from
		body.DateTo = &to
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "dateFrom")
	})

	t.Run("invalid status item returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		statuses := []api.CreatorApplicationsListRequestStatuses{api.Verification, "ghost"}
		body.Statuses = &statuses
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "ghost")
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, listPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("dictionary categories error returns 500", func(t *testing.T) {
		t.Parallel()
		// Service must return a non-empty page so the handler reaches the
		// dictionary-hydration step. The empty-page short-circuit skips
		// dictionary calls entirely, so an empty page would never surface
		// a dictionary error.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{{ID: "11111111-2222-3333-4444-555555555555", Status: domain.CreatorApplicationStatusVerification}},
			Total: 1, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return(nil, errors.New("dict down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, listPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("dictionary cities error returns 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{{ID: "11111111-2222-3333-4444-555555555555", Status: domain.CreatorApplicationStatusVerification}},
			Total: 1, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return(nil, errors.New("dict down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, listPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid status from service triggers 500", func(t *testing.T) {
		t.Parallel()
		// Domain status is normally one of seven canonical strings — if a future
		// migration leaves a stale value, the mapper must surface a 500 rather
		// than silently emit an empty enum to the client.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{{ID: "11111111-2222-3333-4444-555555555555", Status: "ghost"}},
			Total: 1, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, nil)

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, listPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid uuid from service triggers 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{{ID: "not-a-uuid", Status: domain.CreatorApplicationStatusVerification}},
			Total: 1, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, nil)

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, listPath)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("happy path: forwards filters, hydrates, returns shape", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationListInput) {
				captured = in
			}).
			Return(&domain.CreatorApplicationListPage{
				Items: []*domain.CreatorApplicationListItem{{
					ID: appID.String(), Status: domain.CreatorApplicationStatusVerification,
					LastName: "Муратова", FirstName: "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					BirthDate:  birth, CityCode: "almaty",
					Categories: []string{"fashion", "beauty"},
					Socials: []domain.CreatorApplicationDetailSocial{
						{Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
					},
					TelegramLinked: true,
					CreatedAt:      created, UpdatedAt: updated,
				}},
				Total: 1, Page: 1, PerPage: 20,
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return([]domain.DictionaryEntry{
				{Code: "beauty", Name: "Красота", SortOrder: 10},
				{Code: "fashion", Name: "Мода", SortOrder: 20},
			}, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{
				{Code: "almaty", Name: "Алматы", SortOrder: 100},
			}, nil)

		body := validBody()
		statuses := []api.CreatorApplicationsListRequestStatuses{api.Verification, api.Moderation}
		cities := []string{"almaty", "astana"}
		categories := []string{"beauty"}
		body.Statuses = &statuses
		body.Cities = &cities
		body.Categories = &categories
		body.DateFrom = &dateFrom
		body.DateTo = &dateTo
		body.AgeFrom = pointer.ToInt(18)
		body.AgeTo = pointer.ToInt(40)
		body.TelegramLinked = pointer.ToBool(true)
		body.Search = pointer.ToString("aidana")

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)

		// Captured input — the handler hands the trimmed/typed values to the service.
		require.Equal(t, domain.CreatorApplicationListInput{
			Statuses:       []string{"verification", "moderation"},
			Cities:         []string{"almaty", "astana"},
			Categories:     []string{"beauty"},
			DateFrom:       &dateFrom,
			DateTo:         &dateTo,
			AgeFrom:        pointer.ToInt(18),
			AgeTo:          pointer.ToInt(40),
			TelegramLinked: pointer.ToBool(true),
			Search:         "aidana",
			Sort:           "created_at",
			Order:          "desc",
			Page:           1,
			PerPage:        20,
		}, captured)

		// Response shape — categories sorted by (sortOrder, code), city resolved.
		require.Equal(t, api.CreatorApplicationsListResult{
			Data: api.CreatorApplicationsListData{
				Items: []api.CreatorApplicationListItem{{
					Id:         appID,
					Status:     api.CreatorApplicationListItemStatusVerification,
					LastName:   "Муратова",
					FirstName:  "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					BirthDate:  openapi_types.Date{Time: birth},
					City:       api.CreatorApplicationDetailCity{Code: "almaty", Name: "Алматы", SortOrder: 100},
					Categories: []api.CreatorApplicationDetailCategory{
						{Code: "beauty", Name: "Красота", SortOrder: 10},
						{Code: "fashion", Name: "Мода", SortOrder: 20},
					},
					Socials: []api.CreatorApplicationDetailSocial{
						{Platform: api.Instagram, Handle: "aidana"},
					},
					TelegramLinked: true,
					CreatedAt:      created,
					UpdatedAt:      updated,
				}},
				Total:   1,
				Page:    1,
				PerPage: 20,
			},
		}, resp)
	})

	t.Run("happy empty: zero items skips dictionary round-trips", func(t *testing.T) {
		t.Parallel()
		// Empty pages do not need dictionary names — the handler short-
		// circuits to avoid two unnecessary round-trips per poll. The mock
		// dictionary service is wired but configured WITHOUT EXPECT — any
		// call would fail the test (mockery's strict mode).
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
			Items: nil, Total: 0, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, validBody())
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CreatorApplicationsListResult{
			Data: api.CreatorApplicationsListData{
				Items:   []api.CreatorApplicationListItem{},
				Total:   0,
				Page:    1,
				PerPage: 20,
			},
		}, resp)
	})

	t.Run("validation: search > 128 runes returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Search = pointer.ToString(strings.Repeat("a", domain.CreatorApplicationListSearchMaxLen+1))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
		require.Contains(t, resp.Error.Message, "search")
	})

	t.Run("validation: search trims whitespace before length check", func(t *testing.T) {
		t.Parallel()
		// Padding of pure whitespace must not push the search into rejection
		// — trimming happens first, then the length test.
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationListInput) {
				captured = in
			}).Return(&domain.CreatorApplicationListPage{Total: 0, Page: 1, PerPage: 20}, nil)
		dict := mocks.NewMockDictionaryService(t)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Search = pointer.ToString("    aidana    ")
		w, _ := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "aidana", captured.Search)
	})

	t.Run("validation: ageFrom < 0 returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.AgeFrom = pointer.ToInt(-1)
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "ageFrom")
	})

	t.Run("validation: ageTo > 120 returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.AgeTo = pointer.ToInt(domain.CreatorApplicationListAgeMax + 1)
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "ageTo")
	})

	t.Run("validation: page above max returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		body.Page = domain.CreatorApplicationListPageMax + 1
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "page")
	})

	t.Run("validation: dateFrom = zero time returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		zero := time.Time{}
		body.DateFrom = &zero
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "dateFrom")
	})

	t.Run("validation: cities[] empty element returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		bad := []string{"almaty", ""}
		body.Cities = &bad
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "cities")
	})

	t.Run("validation: cities[] item over 64 chars returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		bad := []string{strings.Repeat("a", domain.CreatorApplicationListCityCodeMaxLen+1)}
		body.Cities = &bad
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "cities")
	})

	t.Run("validation: categories[] over array max returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		body := validBody()
		bad := make([]string, domain.CreatorApplicationListFilterArrayMax+1)
		for i := range bad {
			bad[i] = "x"
		}
		body.Categories = &bad
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, resp.Error.Message, "categories")
	})

	t.Run("validation: cities[] dedup keeps order, drops duplicates", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var captured domain.CreatorApplicationListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorApplicationListInput) {
				captured = in
			}).Return(&domain.CreatorApplicationListPage{Total: 0, Page: 1, PerPage: 20}, nil)
		dict := mocks.NewMockDictionaryService(t)
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		body := validBody()
		dup := []string{"almaty", "  astana  ", "almaty"}
		body.Cities = &dup
		w, _ := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, []string{"almaty", "astana"}, captured.Cities)
	})
}

func TestEscapeLikeWildcards_Handler(t *testing.T) {
	// This is the handler-side "validateCodeArray + validateSearch + dedup
	// behaviour" snapshot. The repo-level escape lives in the repo package,
	// but the handler also exercises whitespace trim + dedup.
	t.Parallel()
	t.Run("nil pointer returns nil", func(t *testing.T) {
		t.Parallel()
		out, err := validateCodeArray("cities", nil, 64)
		require.NoError(t, err)
		require.Nil(t, out)
	})
	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		empty := []string{}
		out, err := validateCodeArray("cities", &empty, 64)
		require.NoError(t, err)
		require.Nil(t, out)
	})
	t.Run("trims and dedups", func(t *testing.T) {
		t.Parallel()
		in := []string{"a", "  a  ", " b "}
		out, err := validateCodeArray("cities", &in, 64)
		require.NoError(t, err)
		require.Equal(t, []string{"a", "b"}, out)
	})
}
