package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	return NewServer(nil, nil, authz, nil, nil, creators, nil, nil, ServerConfig{Version: "test-version"}, log)
}

// serverWithAuthzCreatorAndDict wires the trio the POST /creators/list path
// needs: authz + creator (list-aware service) + dictionary. Other fields stay
// nil — the list endpoint never touches them.
func serverWithAuthzCreatorAndDict(t *testing.T, authz AuthzService, creators CreatorService, dict DictionaryService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, authz, nil, nil, creators, nil, dict, ServerConfig{Version: "test-version"}, log)
}

// validCreatorListBody returns the canonical creator-list-request body shared
// by the list endpoint test cases. Each case mutates one field to exercise
// its branch.
func validCreatorListBody() api.CreatorsListRequest {
	return api.CreatorsListRequest{
		Sort:    api.CreatorListSortFieldCreatedAt,
		Order:   api.Desc,
		Page:    1,
		PerPage: 20,
	}
}

// runCreatorListValidation422 wires a fresh authz mock that approves the call,
// applies `mutate` to a valid list body, and asserts the handler answers 422
// with CodeValidation and a message containing wantFragment.
func runCreatorListValidation422(t *testing.T, mutate func(*api.CreatorsListRequest), wantFragment string) {
	t.Helper()
	authz := mocks.NewMockAuthzService(t)
	authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)

	body := validCreatorListBody()
	mutate(&body)

	router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
	w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/list", body)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Equal(t, domain.CodeValidation, resp.Error.Code)
	if wantFragment != "" {
		require.Contains(t, resp.Error.Message, wantFragment)
	}
}

// runCreatorListInternal500 wires authz+creator+dict+logger mocks via the
// supplied configurators and asserts the handler answers 500 for the given
// body. Mirrors runListInternal500 from creator_application_test.go.
func runCreatorListInternal500(t *testing.T,
	body api.CreatorsListRequest,
	configureCreator func(*mocks.MockCreatorService),
	configureDict func(*mocks.MockDictionaryService),
) {
	t.Helper()
	authz := mocks.NewMockAuthzService(t)
	authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)

	var creator *mocks.MockCreatorService
	if configureCreator != nil {
		creator = mocks.NewMockCreatorService(t)
		configureCreator(creator)
	}
	var dict *mocks.MockDictionaryService
	if configureDict != nil {
		dict = mocks.NewMockDictionaryService(t)
		configureDict(dict)
	}

	log := logmocks.NewMockLogger(t)
	expectHandlerUnexpectedErrorLog(log, "/creators/list")

	var creatorIface CreatorService
	if creator != nil {
		creatorIface = creator
	}
	var dictIface DictionaryService
	if dict != nil {
		dictIface = dict
	}
	router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creatorIface, dictIface, log))
	w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/list", body)
	require.Equal(t, http.StatusInternalServerError, w.Code)
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

func TestServer_ListCreators(t *testing.T) {
	t.Parallel()

	const listPath = "/creators/list"

	t.Run("forbidden for manager — service is not consulted", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, nil, nil, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, listPath, validCreatorListBody())
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("invalid body returns 422 (strict-server decode error)", func(t *testing.T) {
		t.Parallel()
		// strict-server intercepts JSON decode failures BEFORE the handler runs,
		// so authz never sees this request — the wrapper short-circuits to 422.
		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, nil, nil, nil, logmocks.NewMockLogger(t)))
		req := httptest.NewRequest(http.MethodPost, listPath, bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	validationCases := []struct {
		name     string
		mutate   func(*api.CreatorsListRequest)
		fragment string
	}{
		{"invalid sort returns 422", func(b *api.CreatorsListRequest) { b.Sort = "rating" }, "sort"},
		{"invalid order returns 422", func(b *api.CreatorsListRequest) { b.Order = "random" }, "order"},
		{"page below 1 returns 422", func(b *api.CreatorsListRequest) { b.Page = 0 }, "page"},
		{"perPage below 1 returns 422", func(b *api.CreatorsListRequest) { b.PerPage = 0 }, "perPage"},
		{"perPage above 200 returns 422", func(b *api.CreatorsListRequest) { b.PerPage = 201 }, "perPage"},
		{"ageFrom > ageTo returns 422", func(b *api.CreatorsListRequest) {
			b.AgeFrom = pointer.ToInt(40)
			b.AgeTo = pointer.ToInt(20)
		}, "ageFrom"},
		{"dateFrom after dateTo returns 422", func(b *api.CreatorsListRequest) {
			from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			b.DateFrom = &from
			b.DateTo = &to
		}, "dateFrom"},
		{"validation: search > 128 runes returns 422", func(b *api.CreatorsListRequest) {
			b.Search = pointer.ToString(strings.Repeat("a", domain.CreatorListSearchMaxLen+1))
		}, "search"},
		{"validation: ageFrom < 0 returns 422", func(b *api.CreatorsListRequest) {
			b.AgeFrom = pointer.ToInt(-1)
		}, "ageFrom"},
		{"validation: ageTo > 120 returns 422", func(b *api.CreatorsListRequest) {
			b.AgeTo = pointer.ToInt(domain.CreatorListAgeMax + 1)
		}, "ageTo"},
		{"validation: page above max returns 422", func(b *api.CreatorsListRequest) {
			b.Page = domain.CreatorListPageMax + 1
		}, "page"},
		{"validation: dateFrom = zero time returns 422", func(b *api.CreatorsListRequest) {
			zero := time.Time{}
			b.DateFrom = &zero
		}, "dateFrom"},
		{"validation: cities[] empty element returns 422", func(b *api.CreatorsListRequest) {
			bad := []string{"almaty", ""}
			b.Cities = &bad
		}, "cities"},
		{"validation: cities[] item over 64 chars returns 422", func(b *api.CreatorsListRequest) {
			bad := []string{strings.Repeat("a", domain.CreatorListCityCodeMaxLen+1)}
			b.Cities = &bad
		}, "cities"},
		{"validation: categories[] over array max returns 422", func(b *api.CreatorsListRequest) {
			bad := make([]string, domain.CreatorListFilterArrayMax+1)
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
			runCreatorListValidation422(t, tc.mutate, tc.fragment)
		})
	}

	nonEmptyPage := func() *domain.CreatorListPage {
		return &domain.CreatorListPage{
			Items: []*domain.CreatorListItem{
				{ID: "11111111-2222-3333-4444-555555555555"},
			},
			Total: 1, Page: 1, PerPage: 20,
		}
	}

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		runCreatorListInternal500(t, validCreatorListBody(), func(c *mocks.MockCreatorService) {
			c.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("db down"))
		}, nil)
	})

	t.Run("dictionary categories error returns 500", func(t *testing.T) {
		t.Parallel()
		runCreatorListInternal500(t, validCreatorListBody(),
			func(c *mocks.MockCreatorService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(nonEmptyPage(), nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, errors.New("dict down"))
			})
	})

	t.Run("dictionary cities error returns 500", func(t *testing.T) {
		t.Parallel()
		runCreatorListInternal500(t, validCreatorListBody(),
			func(c *mocks.MockCreatorService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(nonEmptyPage(), nil)
			},
			func(d *mocks.MockDictionaryService) {
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return(nil, nil)
				d.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, errors.New("dict down"))
			})
	})

	t.Run("invalid uuid from service triggers 500", func(t *testing.T) {
		t.Parallel()
		runCreatorListInternal500(t, validCreatorListBody(),
			func(c *mocks.MockCreatorService) {
				c.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorListPage{
					Items: []*domain.CreatorListItem{{ID: "not-a-uuid"}},
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
		creatorID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		var captured domain.CreatorListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorListInput) {
				captured = in
			}).
			Return(&domain.CreatorListPage{
				Items: []*domain.CreatorListItem{{
					ID:               creatorID.String(),
					LastName:         "Муратова",
					FirstName:        "Айдана",
					MiddleName:       pointer.ToString("Ивановна"),
					IIN:              "950515312348",
					BirthDate:        birth,
					Phone:            "+77001234567",
					CityCode:         "almaty",
					Categories:       []string{"fashion", "beauty"},
					Socials:          []domain.CreatorListSocial{{Platform: domain.SocialPlatformInstagram, Handle: "aidana"}},
					TelegramUsername: pointer.ToString("aidana_tg"),
					CreatedAt:        created,
					UpdatedAt:        updated,
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

		body := validCreatorListBody()
		cities := []string{"almaty", "astana"}
		categories := []string{"beauty"}
		body.Cities = &cities
		body.Categories = &categories
		body.DateFrom = &dateFrom
		body.DateTo = &dateTo
		body.AgeFrom = pointer.ToInt(18)
		body.AgeTo = pointer.ToInt(40)
		body.Search = pointer.ToString("aidana")

		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)

		require.Equal(t, domain.CreatorListInput{
			Cities:     []string{"almaty", "astana"},
			Categories: []string{"beauty"},
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
			AgeFrom:    pointer.ToInt(18),
			AgeTo:      pointer.ToInt(40),
			Search:     "aidana",
			Sort:       "created_at",
			Order:      "desc",
			Page:       1,
			PerPage:    20,
		}, captured)

		require.Equal(t, api.CreatorsListResult{
			Data: api.CreatorsListData{
				Items: []api.CreatorListItem{{
					Id:         creatorID,
					LastName:   "Муратова",
					FirstName:  "Айдана",
					MiddleName: pointer.ToString("Ивановна"),
					Iin:        "950515312348",
					BirthDate:  openapi_types.Date{Time: birth},
					Phone:      "+77001234567",
					City:       api.DictionaryItem{Code: "almaty", Name: "Алматы", SortOrder: 100},
					Categories: []api.DictionaryItem{
						{Code: "beauty", Name: "Красота", SortOrder: 10},
						{Code: "fashion", Name: "Мода", SortOrder: 20},
					},
					Socials:          []api.CreatorListSocial{{Platform: api.Instagram, Handle: "aidana"}},
					TelegramUsername: pointer.ToString("aidana_tg"),
					CreatedAt:        created,
					UpdatedAt:        updated,
				}},
				Total:   1,
				Page:    1,
				PerPage: 20,
			},
		}, resp)
	})

	t.Run("happy empty: zero items skips dictionary round-trips", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorListPage{
			Items: nil, Total: 0, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)

		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorsListResult](t, router, http.MethodPost, listPath, validCreatorListBody())
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.CreatorsListResult{
			Data: api.CreatorsListData{
				Items:   []api.CreatorListItem{},
				Total:   0,
				Page:    1,
				PerPage: 20,
			},
		}, resp)
	})

	t.Run("validation: search trims whitespace before length check", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		var captured domain.CreatorListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorListInput) {
				captured = in
			}).Return(&domain.CreatorListPage{Total: 0, Page: 1, PerPage: 20}, nil)
		dict := mocks.NewMockDictionaryService(t)
		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		body := validCreatorListBody()
		body.Search = pointer.ToString("    aidana    ")
		w, _ := doJSON[api.CreatorsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "aidana", captured.Search)
	})

	t.Run("validation: cities[] dedup keeps order, drops duplicates", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		var captured domain.CreatorListInput
		creator.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.CreatorListInput) {
				captured = in
			}).Return(&domain.CreatorListPage{Total: 0, Page: 1, PerPage: 20}, nil)
		dict := mocks.NewMockDictionaryService(t)
		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		body := validCreatorListBody()
		dup := []string{"almaty", "  astana  ", "almaty"}
		body.Cities = &dup
		w, _ := doJSON[api.CreatorsListResult](t, router, http.MethodPost, listPath, body)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, []string{"almaty", "astana"}, captured.Cities)
	})

	t.Run("happy: deactivated city falls back to (code, code, 0)", func(t *testing.T) {
		t.Parallel()
		creatorID := uuid.MustParse("22222222-3333-4444-5555-666666666666")
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanViewCreators(mock.Anything).Return(nil)
		creator := mocks.NewMockCreatorService(t)
		creator.EXPECT().List(mock.Anything, mock.Anything).Return(&domain.CreatorListPage{
			Items: []*domain.CreatorListItem{{
				ID:         creatorID.String(),
				LastName:   "Муратова",
				FirstName:  "Айдана",
				IIN:        "950515312348",
				BirthDate:  birth,
				Phone:      "+77001234567",
				CityCode:   "retired_city",
				Categories: []string{"retired_niche"},
				CreatedAt:  created,
				UpdatedAt:  updated,
			}},
			Total: 1, Page: 1, PerPage: 20,
		}, nil)
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return([]domain.DictionaryEntry{}, nil)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).
			Return([]domain.DictionaryEntry{}, nil)

		router := newTestRouter(t, serverWithAuthzCreatorAndDict(t, authz, creator, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorsListResult](t, router, http.MethodPost, listPath, validCreatorListBody())
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Items, 1)
		require.Equal(t, api.DictionaryItem{Code: "retired_city", Name: "retired_city", SortOrder: 0}, resp.Data.Items[0].City)
		require.Equal(t, []api.DictionaryItem{{Code: "retired_niche", Name: "retired_niche", SortOrder: 0}}, resp.Data.Items[0].Categories)
	})
}

func TestValidateCreatorAgeBound(t *testing.T) {
	t.Parallel()
	t.Run("nil pointer returns nil", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateCreatorAgeBound("ageFrom", nil))
	})
	t.Run("below min returns 422", func(t *testing.T) {
		t.Parallel()
		v := domain.CreatorListAgeMin - 1
		err := validateCreatorAgeBound("ageFrom", &v)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ageFrom")
	})
	t.Run("above max returns 422", func(t *testing.T) {
		t.Parallel()
		v := domain.CreatorListAgeMax + 1
		err := validateCreatorAgeBound("ageTo", &v)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ageTo")
	})
	t.Run("within range returns nil", func(t *testing.T) {
		t.Parallel()
		v := 25
		require.NoError(t, validateCreatorAgeBound("ageFrom", &v))
	})
}

func TestValidateCreatorSearch(t *testing.T) {
	t.Parallel()
	t.Run("nil returns empty", func(t *testing.T) {
		t.Parallel()
		out, err := validateCreatorSearch(nil)
		require.NoError(t, err)
		require.Equal(t, "", out)
	})
	t.Run("trims whitespace", func(t *testing.T) {
		t.Parallel()
		s := "   aidana   "
		out, err := validateCreatorSearch(&s)
		require.NoError(t, err)
		require.Equal(t, "aidana", out)
	})
	t.Run("rejects too long", func(t *testing.T) {
		t.Parallel()
		s := strings.Repeat("a", domain.CreatorListSearchMaxLen+1)
		_, err := validateCreatorSearch(&s)
		require.Error(t, err)
		require.Contains(t, err.Error(), "search")
	})
	t.Run("trim before length check", func(t *testing.T) {
		t.Parallel()
		// Padding with whitespace must not push the search over the limit.
		s := strings.Repeat(" ", 50) + "aidana" + strings.Repeat(" ", 100)
		out, err := validateCreatorSearch(&s)
		require.NoError(t, err)
		require.Equal(t, "aidana", out)
	})
	t.Run("rejects NUL byte", func(t *testing.T) {
		t.Parallel()
		s := "ai\x00dana"
		_, err := validateCreatorSearch(&s)
		require.Error(t, err)
		require.Contains(t, err.Error(), "search")
	})
}

func TestValidateCreatorCodeArray(t *testing.T) {
	t.Parallel()
	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		out, err := validateCreatorCodeArray("cities", nil, 64)
		require.NoError(t, err)
		require.Nil(t, out)
	})
	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		empty := []string{}
		out, err := validateCreatorCodeArray("cities", &empty, 64)
		require.NoError(t, err)
		require.Nil(t, out)
	})
	t.Run("trims and dedups", func(t *testing.T) {
		t.Parallel()
		in := []string{"a", "  a  ", " b "}
		out, err := validateCreatorCodeArray("cities", &in, 64)
		require.NoError(t, err)
		require.Equal(t, []string{"a", "b"}, out)
	})
	t.Run("rejects empty element", func(t *testing.T) {
		t.Parallel()
		in := []string{"a", ""}
		_, err := validateCreatorCodeArray("cities", &in, 64)
		require.ErrorContains(t, err, "cities")
	})
	t.Run("rejects too long element", func(t *testing.T) {
		t.Parallel()
		in := []string{strings.Repeat("a", 65)}
		_, err := validateCreatorCodeArray("cities", &in, 64)
		require.ErrorContains(t, err, "cities")
	})
	t.Run("rejects oversize array", func(t *testing.T) {
		t.Parallel()
		in := make([]string, domain.CreatorListFilterArrayMax+1)
		for i := range in {
			in[i] = "x"
		}
		_, err := validateCreatorCodeArray("categories", &in, 64)
		require.ErrorContains(t, err, "categories")
	})
}
