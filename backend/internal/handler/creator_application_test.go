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
		City:       "Алматы",
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
			LastName:         "Муратова",
			FirstName:        "Айдана",
			MiddleName:       pointer.ToString("Ивановна"),
			IIN:              "950515312348",
			Phone:            "+77001234567",
			City:             "Алматы",
			Address:          pointer.ToString("ул. Абая 1"),
			CategoryCodes:    []string{"beauty", "fashion"},
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
		oversized := strings.Repeat("x", maxUserAgentLength+128)

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
		require.Len(t, captured.UserAgent, maxUserAgentLength)
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

// newTestRouterWithClientIP wires the ClientIP middleware the same way
// cmd/api does, so handler tests can verify that IPAddress flows from
// r.RemoteAddr into the service input.
func newTestRouterWithClientIP(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	r.Use(middleware.ClientIP)
	api.HandlerWithOptions(s, api.ChiServerOptions{
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
		// short-circuits before the maptter runs, so dict stays nil.
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
				City:              "almaty",
				Address:           pointer.ToString("ул. Абая 1"),
				CategoryOtherText: pointer.ToString("Авторские ASMR"),
				Status:            domain.CreatorApplicationStatusPending,
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
				Status:            api.Pending,
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
				City:       "atyrau", // not present in the active dictionary below
				Address:    pointer.ToString("ул. Абая 1"),
				Status:     domain.CreatorApplicationStatusPending,
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
