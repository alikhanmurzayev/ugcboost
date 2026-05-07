package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
	return NewServer(nil, nil, nil, nil, creator, nil, nil, nil, nil, ServerConfig{
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
		server := NewServer(nil, nil, nil, nil, creator, nil, nil, nil, nil, ServerConfig{
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
	return NewServer(nil, nil, authz, nil, creator, nil, nil, nil, dict, ServerConfig{
		Version:             "test-version",
		TelegramBotUsername: "ugcboost_test_bot",
	}, log)
}

// validListBody returns the canonical list-request body shared by the list
// endpoint test cases. Each case mutates one field to exercise its branch.
func validListBody() api.CreatorApplicationsListRequest {
	return api.CreatorApplicationsListRequest{
		Sort:    api.CreatorApplicationListSortFieldCreatedAt,
		Order:   api.Desc,
		Page:    1,
		PerPage: 20,
	}
}

// runListValidation422 wires a fresh authz mock that approves the call,
// applies `mutate` to a valid list body, and asserts the handler answers 422
// with CodeValidation and a message containing wantFragment. Service and
// dictionary mocks are intentionally nil — validation must reject before they
// are touched.
func runListValidation422(t *testing.T, mutate func(*api.CreatorApplicationsListRequest), wantFragment string) {
	t.Helper()
	authz := mocks.NewMockAuthzService(t)
	authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

	body := validListBody()
	mutate(&body)

	router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
	w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications/list", body)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Equal(t, domain.CodeValidation, resp.Error.Code)
	if wantFragment != "" {
		require.Contains(t, resp.Error.Message, wantFragment)
	}
}

// runListInternal500 wires authz+creator+dict+logger mocks via the supplied
// configurators and asserts the handler answers 500 for the given body. The
// configurator sets up whatever mock expectation triggers the 500 path
// (service error, dictionary error, invalid status from service, etc.) — only
// non-nil mocks are passed to the server.
func runListInternal500(t *testing.T,
	body api.CreatorApplicationsListRequest,
	configureCreator func(*mocks.MockCreatorApplicationService),
	configureDict func(*mocks.MockDictionaryService),
) {
	t.Helper()
	authz := mocks.NewMockAuthzService(t)
	authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(nil)

	var creator *mocks.MockCreatorApplicationService
	if configureCreator != nil {
		creator = mocks.NewMockCreatorApplicationService(t)
		configureCreator(creator)
	}
	var dict *mocks.MockDictionaryService
	if configureDict != nil {
		dict = mocks.NewMockDictionaryService(t)
		configureDict(dict)
	}

	log := logmocks.NewMockLogger(t)
	expectHandlerUnexpectedErrorLog(log, "/creators/applications/list")

	var creatorIface CreatorApplicationService
	if creator != nil {
		creatorIface = creator
	}
	var dictIface DictionaryService
	if dict != nil {
		dictIface = dict
	}
	router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creatorIface, dictIface, log))
	w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications/list", body)
	require.Equal(t, http.StatusInternalServerError, w.Code)
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
					{ID: "aaaaaaaa-1111-1111-1111-111111111111", Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
					{ID: "aaaaaaaa-2222-2222-2222-222222222222", Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
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
				City:              api.DictionaryItem{Code: "almaty", Name: "Алматы", SortOrder: 100},
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            api.Verification,
				CreatedAt:         created,
				UpdatedAt:         updated,
				Categories: []api.DictionaryItem{
					{Code: "beauty", Name: "Красота", SortOrder: 10},
					{Code: "fashion", Name: "Мода", SortOrder: 20},
				},
				Socials: []api.CreatorApplicationDetailSocial{
					{Id: uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"), Platform: api.Instagram, Handle: "aidana"},
					{Id: uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"), Platform: api.Tiktok, Handle: "aidana_tt"},
				},
				Consents: []api.CreatorApplicationDetailConsent{
					{ConsentType: api.Processing, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.ThirdParty, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.CrossBorder, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
					{ConsentType: api.Terms, AcceptedAt: acceptedAt, DocumentVersion: "2026-04-20", IpAddress: "127.0.0.1", UserAgent: "ua/1"},
				},
				TelegramBotUrl: "https://t.me/ugcboost_test_bot?start=" + appID.String(),
			},
		}, resp)
	})

	t.Run("exposes telegramBotUrl built from configured bot username", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{
				ID:       appID.String(),
				LastName: "Тест", FirstName: "Тест",
				IIN:       "950515312348",
				BirthDate: birth, Phone: "+77001234567",
				CityCode:  "almaty",
				Status:    domain.CreatorApplicationStatusVerification,
				CreatedAt: created, UpdatedAt: created,
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{{Code: "almaty", Name: "Алматы", SortOrder: 100}}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorApplicationResult](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "https://t.me/ugcboost_test_bot?start="+appID.String(), resp.Data.TelegramBotUrl)
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
		require.Equal(t, []api.DictionaryItem{
			{Code: "vintage_asmr", Name: "vintage_asmr", SortOrder: 0},
			{Code: "beauty", Name: "Красота", SortOrder: 10},
		}, resp.Data.Categories)
		require.Equal(t, api.DictionaryItem{
			Code: "atyrau", Name: "atyrau", SortOrder: 0,
		}, resp.Data.City)
	})

	t.Run("rejected app maps rejection block", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		actorID := uuid.MustParse("aaaa1111-1111-1111-1111-111111111111")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		rejectedAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{
				ID:       appID.String(),
				LastName: "Тест", FirstName: "Тест",
				IIN:       "950515312348",
				BirthDate: birth, Phone: "+77001234567",
				CityCode:  "almaty",
				Status:    domain.CreatorApplicationStatusRejected,
				CreatedAt: created, UpdatedAt: created,
				Rejection: &domain.CreatorApplicationRejection{
					FromStatus:       domain.CreatorApplicationStatusModeration,
					RejectedAt:       rejectedAt,
					RejectedByUserID: actorID.String(),
				},
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{{Code: "almaty", Name: "Алматы", SortOrder: 100}}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorApplicationResult](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.Rejected, resp.Data.Status)
		require.NotNil(t, resp.Data.Rejection)
		require.Equal(t, &api.CreatorApplicationRejection{
			FromStatus:       api.Moderation,
			RejectedAt:       rejectedAt,
			RejectedByUserId: actorID,
		}, resp.Data.Rejection)
	})

	t.Run("non-rejected app omits rejection block", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().GetByID(mock.Anything, appID.String()).
			Return(&domain.CreatorApplicationDetail{
				ID:       appID.String(),
				LastName: "Тест", FirstName: "Тест",
				IIN:       "950515312348",
				BirthDate: birth, Phone: "+77001234567",
				CityCode:  "almaty",
				Status:    domain.CreatorApplicationStatusVerification,
				CreatedAt: created, UpdatedAt: created,
			}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{{Code: "almaty", Name: "Алматы", SortOrder: 100}}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.GetCreatorApplicationResult](t, router, http.MethodGet, appPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Nil(t, resp.Data.Rejection)
	})
}

func TestMapCreatorApplicationStatusToAPI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		domainValue string
		apiValue    api.CreatorApplicationStatus
	}{
		{domain.CreatorApplicationStatusVerification, api.Verification},
		{domain.CreatorApplicationStatusModeration, api.Moderation},
		{domain.CreatorApplicationStatusApproved, api.Approved},
		{domain.CreatorApplicationStatusRejected, api.Rejected},
		{domain.CreatorApplicationStatusWithdrawn, api.Withdrawn},
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
		empty := []api.CreatorApplicationStatus{}
		out, err := apiListStatusesToDomain(&empty)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("known statuses dedup preserved order", func(t *testing.T) {
		t.Parallel()
		in := []api.CreatorApplicationStatus{
			api.Verification, api.Moderation, api.Verification,
		}
		out, err := apiListStatusesToDomain(&in)
		require.NoError(t, err)
		require.Equal(t, []string{"verification", "moderation"}, out)
	})

	t.Run("unknown status surfaces 422", func(t *testing.T) {
		t.Parallel()
		bad := []api.CreatorApplicationStatus{api.Verification, "ghost"}
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

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListCreatorApplications(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validListBody())
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

	// Validation cases — all share the same harness: authz approves, the
	// handler-side validators reject before the service is ever called.
	validationCases := []struct {
		name     string
		mutate   func(*api.CreatorApplicationsListRequest)
		fragment string
	}{
		{"invalid sort returns 422", func(b *api.CreatorApplicationsListRequest) { b.Sort = "rating" }, "sort"},
		{"invalid order returns 422", func(b *api.CreatorApplicationsListRequest) { b.Order = "random" }, "order"},
		{"page below 1 returns 422", func(b *api.CreatorApplicationsListRequest) { b.Page = 0 }, "page"},
		{"perPage below 1 returns 422", func(b *api.CreatorApplicationsListRequest) { b.PerPage = 0 }, "perPage"},
		{"perPage above 200 returns 422", func(b *api.CreatorApplicationsListRequest) { b.PerPage = 201 }, ""},
		{"ageFrom > ageTo returns 422", func(b *api.CreatorApplicationsListRequest) {
			b.AgeFrom = pointer.ToInt(40)
			b.AgeTo = pointer.ToInt(20)
		}, "ageFrom"},
		{"dateFrom after dateTo returns 422", func(b *api.CreatorApplicationsListRequest) {
			from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			b.DateFrom = &from
			b.DateTo = &to
		}, "dateFrom"},
		{"invalid status item returns 422", func(b *api.CreatorApplicationsListRequest) {
			statuses := []api.CreatorApplicationStatus{api.Verification, "ghost"}
			b.Statuses = &statuses
		}, "ghost"},
		{"validation: search > 128 runes returns 422", func(b *api.CreatorApplicationsListRequest) {
			b.Search = pointer.ToString(strings.Repeat("a", domain.CreatorApplicationListSearchMaxLen+1))
		}, "search"},
		{"validation: ageFrom < 0 returns 422", func(b *api.CreatorApplicationsListRequest) {
			b.AgeFrom = pointer.ToInt(-1)
		}, "ageFrom"},
		{"validation: ageTo > 120 returns 422", func(b *api.CreatorApplicationsListRequest) {
			b.AgeTo = pointer.ToInt(domain.CreatorApplicationListAgeMax + 1)
		}, "ageTo"},
		{"validation: page above max returns 422", func(b *api.CreatorApplicationsListRequest) {
			b.Page = domain.CreatorApplicationListPageMax + 1
		}, "page"},
		{"validation: dateFrom = zero time returns 422", func(b *api.CreatorApplicationsListRequest) {
			zero := time.Time{}
			b.DateFrom = &zero
		}, "dateFrom"},
		{"validation: cities[] empty element returns 422", func(b *api.CreatorApplicationsListRequest) {
			bad := []string{"almaty", ""}
			b.Cities = &bad
		}, "cities"},
		{"validation: cities[] item over 64 chars returns 422", func(b *api.CreatorApplicationsListRequest) {
			bad := []string{strings.Repeat("a", domain.CreatorApplicationListCityCodeMaxLen+1)}
			b.Cities = &bad
		}, "cities"},
		{"validation: categories[] over array max returns 422", func(b *api.CreatorApplicationsListRequest) {
			bad := make([]string, domain.CreatorApplicationListFilterArrayMax+1)
			for i := range bad {
				bad[i] = "x"
			}
			b.Categories = &bad
		}, "categories"},
	}
	for _, tc := range validationCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runListValidation422(t, tc.mutate, tc.fragment)
		})
	}

	// 500-class cases — service or dictionary error path. nonEmptyPage builds
	// the canonical "one item, dictionaries called" page so the dictionary
	// branch is reachable (the empty-page short-circuit would skip it).
	nonEmptyPage := func() *domain.CreatorApplicationListPage {
		return &domain.CreatorApplicationListPage{
			Items: []*domain.CreatorApplicationListItem{
				{ID: "11111111-2222-3333-4444-555555555555", Status: domain.CreatorApplicationStatusVerification},
			},
			Total: 1, Page: 1, PerPage: 20,
		}
	}

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		runListInternal500(t, validListBody(), func(c *mocks.MockCreatorApplicationService) {
			c.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("db down"))
		}, nil)
	})

	t.Run("dictionary categories error returns 500", func(t *testing.T) {
		t.Parallel()
		runListInternal500(t, validListBody(),
			func(c *mocks.MockCreatorApplicationService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(nonEmptyPage(), nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, errors.New("dict down"))
			})
	})

	t.Run("dictionary cities error returns 500", func(t *testing.T) {
		t.Parallel()
		runListInternal500(t, validListBody(),
			func(c *mocks.MockCreatorApplicationService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(nonEmptyPage(), nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, errors.New("dict down"))
			})
	})

	t.Run("invalid status from service triggers 500", func(t *testing.T) {
		t.Parallel()
		// Domain status is normally one of seven canonical strings — if a future
		// migration leaves a stale value, the mapper must surface a 500 rather
		// than silently emit an empty enum to the client.
		runListInternal500(t, validListBody(),
			func(c *mocks.MockCreatorApplicationService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
					Items: []*domain.CreatorApplicationListItem{{ID: "11111111-2222-3333-4444-555555555555", Status: "ghost"}},
					Total: 1, Page: 1, PerPage: 20,
				}, nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, nil)
			})
	})

	t.Run("invalid uuid from service triggers 500", func(t *testing.T) {
		t.Parallel()
		runListInternal500(t, validListBody(),
			func(c *mocks.MockCreatorApplicationService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorApplicationListPage{
					Items: []*domain.CreatorApplicationListItem{{ID: "not-a-uuid", Status: domain.CreatorApplicationStatusVerification}},
					Total: 1, Page: 1, PerPage: 20,
				}, nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, nil)
			})
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
						{ID: "bbbbbbbb-1111-1111-1111-111111111111", Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
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

		body := validListBody()
		statuses := []api.CreatorApplicationStatus{api.Verification, api.Moderation}
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
					Status:     api.Verification,
					LastName:   "Муратова",
					FirstName:  "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					BirthDate:  openapi_types.Date{Time: birth},
					City:       api.DictionaryItem{Code: "almaty", Name: "Алматы", SortOrder: 100},
					Categories: []api.DictionaryItem{
						{Code: "beauty", Name: "Красота", SortOrder: 10},
						{Code: "fashion", Name: "Мода", SortOrder: 20},
					},
					Socials: []api.CreatorApplicationDetailSocial{
						{Id: uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111"), Platform: api.Instagram, Handle: "aidana"},
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
		w, resp := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, validListBody())
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
		body := validListBody()
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
		body := validListBody()
		body.Search = pointer.ToString("    aidana    ")
		w, _ := doJSON[api.CreatorApplicationsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "aidana", captured.Search)
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
		body := validListBody()
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

func TestDomainCreatorApplicationDetailSocialToAPI(t *testing.T) {
	t.Parallel()

	const validSocialID = "cccccccc-1111-1111-1111-111111111111"

	t.Run("unverified default — verified=false plus three nils pass through", func(t *testing.T) {
		t.Parallel()
		got, err := domainCreatorApplicationDetailSocialToAPI(domain.CreatorApplicationDetailSocial{
			ID:       validSocialID,
			Platform: domain.SocialPlatformInstagram,
			Handle:   "aidana",
		})
		require.NoError(t, err)
		require.Equal(t, api.CreatorApplicationDetailSocial{
			Id:       uuid.MustParse(validSocialID),
			Platform: api.SocialPlatform(domain.SocialPlatformInstagram),
			Handle:   "aidana",
			Verified: false,
		}, got)
	})

	t.Run("manual verification — every field maps", func(t *testing.T) {
		t.Parallel()
		method := domain.SocialVerificationMethodManual
		adminID := "11111111-2222-3333-4444-555555555555"
		when := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

		got, err := domainCreatorApplicationDetailSocialToAPI(domain.CreatorApplicationDetailSocial{
			ID:               validSocialID,
			Platform:         domain.SocialPlatformInstagram,
			Handle:           "aidana",
			Verified:         true,
			Method:           &method,
			VerifiedByUserID: &adminID,
			VerifiedAt:       &when,
		})
		require.NoError(t, err)
		require.Equal(t, validSocialID, got.Id.String())
		require.True(t, got.Verified)
		require.NotNil(t, got.Method)
		require.Equal(t, api.SocialVerificationMethod(method), *got.Method)
		require.NotNil(t, got.VerifiedByUserId)
		require.Equal(t, adminID, got.VerifiedByUserId.String())
		require.NotNil(t, got.VerifiedAt)
		require.True(t, got.VerifiedAt.Equal(when))
	})

	t.Run("auto verification — verified true with no admin uuid", func(t *testing.T) {
		t.Parallel()
		method := domain.SocialVerificationMethodAuto
		when := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

		got, err := domainCreatorApplicationDetailSocialToAPI(domain.CreatorApplicationDetailSocial{
			ID:         validSocialID,
			Platform:   domain.SocialPlatformInstagram,
			Handle:     "aidana",
			Verified:   true,
			Method:     &method,
			VerifiedAt: &when,
		})
		require.NoError(t, err)
		require.True(t, got.Verified)
		require.NotNil(t, got.Method)
		require.Equal(t, api.SocialVerificationMethod(method), *got.Method)
		require.Nil(t, got.VerifiedByUserId)
		require.NotNil(t, got.VerifiedAt)
	})

	t.Run("invalid social ID — error wraps the bad value", func(t *testing.T) {
		t.Parallel()
		_, err := domainCreatorApplicationDetailSocialToAPI(domain.CreatorApplicationDetailSocial{
			ID:       "not-a-uuid",
			Platform: domain.SocialPlatformInstagram,
			Handle:   "aidana",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "social id")
		require.ErrorContains(t, err, "not-a-uuid")
	})

	t.Run("invalid VerifiedByUserID — error wraps the bad value", func(t *testing.T) {
		t.Parallel()
		bad := "not-a-uuid"
		_, err := domainCreatorApplicationDetailSocialToAPI(domain.CreatorApplicationDetailSocial{
			ID:               validSocialID,
			Platform:         domain.SocialPlatformInstagram,
			Handle:           "aidana",
			VerifiedByUserID: &bad,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "verified_by_user_id")
		require.ErrorContains(t, err, bad)
	})
}

func TestSortDictionaryItem(t *testing.T) {
	t.Parallel()

	t.Run("different sort_order: lower comes first regardless of code", func(t *testing.T) {
		t.Parallel()
		got := sortDictionaryItem(
			api.DictionaryItem{Code: "zzz", SortOrder: 1},
			api.DictionaryItem{Code: "aaa", SortOrder: 5},
		)
		require.Negative(t, got)
	})

	t.Run("equal sort_order falls back to code asc", func(t *testing.T) {
		t.Parallel()
		got := sortDictionaryItem(
			api.DictionaryItem{Code: "aaa", SortOrder: 1},
			api.DictionaryItem{Code: "bbb", SortOrder: 1},
		)
		require.Negative(t, got)
	})

	t.Run("equal in both fields returns zero", func(t *testing.T) {
		t.Parallel()
		got := sortDictionaryItem(
			api.DictionaryItem{Code: "x", SortOrder: 1},
			api.DictionaryItem{Code: "x", SortOrder: 1},
		)
		require.Zero(t, got)
	})
}

func TestServer_VerifyCreatorApplicationSocial(t *testing.T) {
	t.Parallel()

	const (
		appUUID    = "11111111-2222-3333-4444-555555555555"
		socialUUID = "66666666-7777-8888-9999-aaaaaaaaaaaa"
		adminUUID  = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
		path       = "/creators/applications/" + appUUID + "/socials/" + socialUUID + "/verify"
	)

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.BrandManager))
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("unauthenticated — auth middleware short-circuits before handler", func(t *testing.T) {
		t.Parallel()
		// AuthFromScopes wraps the route in production; here we install a
		// minimal stand-in that delegates to respondError to keep the wire
		// shape identical to prod 401s — no hand-crafted JSON literals.
		log := logmocks.NewMockLogger(t)
		r := chi.NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") == "" {
					respondError(w, req, domain.ErrUnauthorized, log)
					return
				}
				next.ServeHTTP(w, req)
			})
		})
		api.HandlerWithOptions(NewStrictAPIHandler(serverWithAuthzAndCreatorAndDict(t, nil, nil, nil, log)), api.ChiServerOptions{
			BaseRouter: r, ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
		})

		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)

		var body api.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, domain.CodeUnauthorized, body.Error.Code)
	})

	t.Run("application not found surfaces as 404 NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, appUUID, socialUUID, adminUUID).
			Return(domain.ErrCreatorApplicationNotFound)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})

	t.Run("social not found surfaces as 404 with social-specific code", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, appUUID, socialUUID, adminUUID).
			Return(domain.ErrCreatorApplicationSocialNotFound)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationSocialNotFound, resp.Error.Code)
	})

	t.Run("already verified surfaces as 409", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, appUUID, socialUUID, adminUUID).
			Return(domain.ErrCreatorApplicationSocialAlreadyVerified)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusConflict, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationSocialAlreadyVerified, resp.Error.Code)
	})

	t.Run("wrong status surfaces as 422 NOT_IN_VERIFICATION", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, appUUID, socialUUID, adminUUID).
			Return(domain.ErrCreatorApplicationNotInVerification)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationNotInVerification, resp.Error.Code)
	})

	t.Run("missing telegram link surfaces as 422 TELEGRAM_NOT_LINKED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, appUUID, socialUUID, adminUUID).
			Return(domain.ErrCreatorApplicationTelegramNotLinked)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationTelegramNotLinked, resp.Error.Code)
	})

	t.Run("happy path: forwards actor and ids, returns empty 200 body", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanVerifyCreatorApplicationSocialManually(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var capturedAppID, capturedSocialID, capturedActor string
		creator.EXPECT().VerifyApplicationSocialManually(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, applicationID, socialID, actorUserID string) {
				capturedAppID = applicationID
				capturedSocialID = socialID
				capturedActor = actorUserID
			}).
			Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.EmptyResult](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.EmptyResult{}, resp)

		require.Equal(t, appUUID, capturedAppID)
		require.Equal(t, socialUUID, capturedSocialID)
		require.Equal(t, adminUUID, capturedActor)
	})
}

func TestServer_RejectCreatorApplication(t *testing.T) {
	t.Parallel()

	const (
		appUUID   = "33333333-4444-5555-6666-777777777777"
		adminUUID = "88888888-9999-aaaa-bbbb-cccccccccccc"
		path      = "/creators/applications/" + appUUID + "/reject"
	)

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRejectCreatorApplication(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.BrandManager))
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("unauthenticated — auth middleware short-circuits before handler", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		r := chi.NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") == "" {
					respondError(w, req, domain.ErrUnauthorized, log)
					return
				}
				next.ServeHTTP(w, req)
			})
		})
		api.HandlerWithOptions(NewStrictAPIHandler(serverWithAuthzAndCreatorAndDict(t, nil, nil, nil, log)), api.ChiServerOptions{
			BaseRouter: r, ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
		})

		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)

		var body api.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, domain.CodeUnauthorized, body.Error.Code)
	})

	t.Run("application not found surfaces as 404 NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRejectCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().RejectApplication(mock.Anything, appUUID, adminUUID).
			Return(domain.ErrCreatorApplicationNotFound)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})

	t.Run("not rejectable surfaces as 422 NOT_REJECTABLE", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRejectCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().RejectApplication(mock.Anything, appUUID, adminUUID).
			Return(domain.ErrCreatorApplicationNotRejectable)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationNotRejectable, resp.Error.Code)
	})

	t.Run("happy path: forwards actor and id, returns empty 200 body", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanRejectCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var capturedAppID, capturedActor string
		creator.EXPECT().RejectApplication(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, applicationID, actorUserID string) {
				capturedAppID = applicationID
				capturedActor = actorUserID
			}).
			Return(nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.EmptyResult](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.EmptyResult{}, resp)

		require.Equal(t, appUUID, capturedAppID)
		require.Equal(t, adminUUID, capturedActor)
	})
}

func TestServer_ApproveCreatorApplication(t *testing.T) {
	t.Parallel()

	const (
		appUUID     = "11111111-1111-1111-1111-111111111111"
		adminUUID   = "22222222-2222-2222-2222-222222222222"
		creatorUUID = "33333333-3333-3333-3333-333333333333"
		path        = "/creators/applications/" + appUUID + "/approve"
	)

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.BrandManager))
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("unauthenticated — auth middleware short-circuits before handler", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		r := chi.NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") == "" {
					respondError(w, req, domain.ErrUnauthorized, log)
					return
				}
				next.ServeHTTP(w, req)
			})
		})
		api.HandlerWithOptions(NewStrictAPIHandler(serverWithAuthzAndCreatorAndDict(t, nil, nil, nil, log)), api.ChiServerOptions{
			BaseRouter: r, ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
		})

		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)

		var body api.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, domain.CodeUnauthorized, body.Error.Code)
	})

	t.Run("application not found surfaces as 404 NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, []string(nil)).
			Return("", domain.ErrCreatorApplicationNotFound)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})

	t.Run("not approvable surfaces as 422 NOT_APPROVABLE", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, []string(nil)).
			Return("", domain.ErrCreatorApplicationNotApprovable)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationNotApprovable, resp.Error.Code)
	})

	t.Run("telegram not linked surfaces as 422 TELEGRAM_NOT_LINKED", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, []string(nil)).
			Return("", domain.ErrCreatorApplicationTelegramNotLinked)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationTelegramNotLinked, resp.Error.Code)
	})

	t.Run("creator already exists surfaces as 422 CREATOR_ALREADY_EXISTS", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, []string(nil)).
			Return("", domain.ErrCreatorAlreadyExists)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorAlreadyExists, resp.Error.Code)
	})

	t.Run("telegram already taken surfaces as 422 CREATOR_TELEGRAM_ALREADY_TAKEN", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, []string(nil)).
			Return("", domain.ErrCreatorTelegramAlreadyTaken)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCreatorTelegramAlreadyTaken, resp.Error.Code)
	})

	t.Run("happy path: forwards actor and id, returns creatorId in 200 body", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var capturedAppID, capturedActor string
		var capturedCampaignIDs []string
		creator.EXPECT().ApproveApplication(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, applicationID, actorUserID string, campaignIDs []string) {
				capturedAppID = applicationID
				capturedActor = actorUserID
				capturedCampaignIDs = campaignIDs
			}).
			Return(creatorUUID, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApprovalResult](t, router, http.MethodPost, path, nil, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, creatorUUID, resp.Data.CreatorId.String())

		require.Equal(t, appUUID, capturedAppID)
		require.Equal(t, adminUUID, capturedActor)
		require.Nil(t, capturedCampaignIDs, "no body → campaignIDs forwarded as nil")
	})

	t.Run("happy path: empty campaignIds in body forwards as nil — old behaviour", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var capturedCampaignIDs []string
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, mock.Anything).
			Run(func(_ context.Context, _, _ string, campaignIDs []string) {
				capturedCampaignIDs = campaignIDs
			}).
			Return(creatorUUID, nil)

		body := api.CreatorApprovalInput{CampaignIds: pointer.To([]openapi_types.UUID{})}
		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CreatorApprovalResult](t, router, http.MethodPost, path, body, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Nil(t, capturedCampaignIDs)
	})

	t.Run("happy path: valid campaignIds pre-validated and forwarded to service", func(t *testing.T) {
		t.Parallel()
		campA := uuid.MustParse("44444444-4444-4444-4444-444444444444")
		campB := uuid.MustParse("55555555-5555-5555-5555-555555555555")

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		campaign := mocks.NewMockCampaignService(t)
		campaign.EXPECT().AssertActiveCampaigns(mock.Anything, []string{campA.String(), campB.String()}).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		var capturedCampaignIDs []string
		creator.EXPECT().ApproveApplication(mock.Anything, appUUID, adminUUID, mock.Anything).
			Run(func(_ context.Context, _, _ string, campaignIDs []string) {
				capturedCampaignIDs = campaignIDs
			}).
			Return(creatorUUID, nil)

		body := api.CreatorApprovalInput{CampaignIds: pointer.To([]openapi_types.UUID{campA, campB})}
		router := newTestRouter(t, serverWithApproveDeps(t, authz, creator, campaign, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.CreatorApprovalResult](t, router, http.MethodPost, path, body, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, []string{campA.String(), campB.String()}, capturedCampaignIDs,
			"normalized campaignIDs forwarded to service in input order")
	})

	t.Run("> 20 campaignIds → 422 CAMPAIGN_IDS_TOO_MANY, service not called", func(t *testing.T) {
		t.Parallel()
		ids := make([]openapi_types.UUID, 21)
		for i := range ids {
			ids[i] = uuid.New()
		}
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		// creator/campaign mocks omitted — handler must reject before calling them.
		body := api.CreatorApprovalInput{CampaignIds: &ids}
		router := newTestRouter(t, serverWithApproveDeps(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, body, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignIdsTooMany, resp.Error.Code)
	})

	t.Run("duplicate campaignIds → 422 CAMPAIGN_IDS_DUPLICATES, service not called", func(t *testing.T) {
		t.Parallel()
		dup := uuid.MustParse("44444444-4444-4444-4444-444444444444")
		other := uuid.MustParse("55555555-5555-5555-5555-555555555555")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		body := api.CreatorApprovalInput{CampaignIds: pointer.To([]openapi_types.UUID{dup, other, dup})}
		router := newTestRouter(t, serverWithApproveDeps(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, body, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignIdsDuplicates, resp.Error.Code)
	})

	t.Run("pre-validation fail → 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD, approve service not called", func(t *testing.T) {
		t.Parallel()
		campA := uuid.MustParse("44444444-4444-4444-4444-444444444444")
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanApproveCreatorApplication(mock.Anything).Return(nil)
		campaign := mocks.NewMockCampaignService(t)
		campaign.EXPECT().AssertActiveCampaigns(mock.Anything, []string{campA.String()}).
			Return(domain.ErrCampaignNotAvailableForAdd)
		// creator mock omitted — handler must reject before calling it.
		body := api.CreatorApprovalInput{CampaignIds: pointer.To([]openapi_types.UUID{campA})}
		router := newTestRouter(t, serverWithApproveDeps(t, authz, nil, campaign, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, path, body, withRole(adminUUID, api.Admin))
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeCampaignNotAvailableForAdd, resp.Error.Code)
		require.Equal(t, domain.ErrCampaignNotAvailableForAdd.Message, resp.Error.Message,
			"422 message must be the actionable text from the domain sentinel — admin reads it inline in the dialog")
	})
}

// serverWithApproveDeps wires authz + creator + campaignSvc for the
// ApproveCreatorApplication tests that drive parseApproveCampaignIDs end-to-
// end. Other server slots are nil — the route is the only thing exercised.
func serverWithApproveDeps(t *testing.T, authz AuthzService, creator CreatorApplicationService, campaignSvc CampaignService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, authz, nil, creator, nil, campaignSvc, nil, nil, ServerConfig{
		Version:             "test-version",
		TelegramBotUsername: "ugcboost_test_bot",
	}, log)
}

func TestServer_GetCreatorApplicationsCounts(t *testing.T) {
	t.Parallel()

	const countsPath = "/creators/applications/counts"

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCreatorApplicationsCounts(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, countsPath, nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("service error surfaces as 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCreatorApplicationsCounts(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Counts(mock.Anything).Return(nil, errors.New("db boom"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, countsPath)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, countsPath, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("admin success — empty map yields empty items", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCreatorApplicationsCounts(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Counts(mock.Anything).Return(map[string]int64{}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApplicationsCountsResult](t, router, http.MethodGet, countsPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CreatorApplicationsCountsResult{
			Data: api.CreatorApplicationsCountsData{
				Items: []api.CreatorApplicationStatusCount{},
			},
		}, resp)
	})

	t.Run("admin success — items sorted alphabetically by status", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanGetCreatorApplicationsCounts(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorApplicationService(t)
		// Map iteration order is non-deterministic — handler MUST sort the
		// resulting slice. Mock returns three statuses out of order; assert
		// the wire payload is alphabetically sorted.
		creator.EXPECT().Counts(mock.Anything).Return(map[string]int64{
			domain.CreatorApplicationStatusVerification: 5,
			domain.CreatorApplicationStatusModeration:   2,
			domain.CreatorApplicationStatusRejected:     1,
		}, nil)

		router := newTestRouter(t, serverWithAuthzAndCreatorAndDict(t, authz, creator, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApplicationsCountsResult](t, router, http.MethodGet, countsPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CreatorApplicationsCountsResult{
			Data: api.CreatorApplicationsCountsData{
				Items: []api.CreatorApplicationStatusCount{
					{Status: api.Moderation, Count: 2},
					{Status: api.Rejected, Count: 1},
					{Status: api.Verification, Count: 5},
				},
			},
		}, resp)
	})
}
