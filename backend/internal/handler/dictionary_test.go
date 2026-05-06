package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func serverWithDictionary(t *testing.T, dict DictionaryService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, nil, nil, nil, nil, nil, dict, ServerConfig{Version: "test-version"}, log)
}

func TestServer_ListDictionary(t *testing.T) {
	t.Parallel()

	t.Run("categories returns 200 with mapped items", func(t *testing.T) {
		t.Parallel()
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return([]domain.DictionaryEntry{
				{Code: "fashion", Name: "Мода / Стиль", SortOrder: 10},
				{Code: "beauty", Name: "Бьюти (макияж, уход)", SortOrder: 20},
			}, nil)

		router := newTestRouter(t, serverWithDictionary(t, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.DictionaryListResult](t, router, http.MethodGet, "/dictionaries/categories", nil)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.DictionaryListResult{
			Data: api.ListDictionaryData{
				Type: "categories",
				Items: []api.DictionaryItem{
					{Code: "fashion", Name: "Мода / Стиль", SortOrder: 10},
					{Code: "beauty", Name: "Бьюти (макияж, уход)", SortOrder: 20},
				},
			},
		}, resp)
	})

	t.Run("cities returns 200 with empty items", func(t *testing.T) {
		t.Parallel()
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCities).Return(nil, nil)

		router := newTestRouter(t, serverWithDictionary(t, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.DictionaryListResult](t, router, http.MethodGet, "/dictionaries/cities", nil)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "cities", resp.Data.Type)
		require.Empty(t, resp.Data.Items)
	})

	t.Run("unknown dictionary type maps to 404", func(t *testing.T) {
		t.Parallel()
		// oapi-codegen leaves the enum value untyped at the wrapper level —
		// it's the service that returns ErrDictionaryUnknownType which the
		// handler maps onto the canonical 404.
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryType("unicorns")).
			Return(nil, domain.ErrDictionaryUnknownType)

		router := newTestRouter(t, serverWithDictionary(t, dict, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/dictionaries/unicorns", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		dict := mocks.NewMockDictionaryService(t)
		dict.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).
			Return(nil, assertError("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/dictionaries/categories")
		router := newTestRouter(t, serverWithDictionary(t, dict, log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/dictionaries/categories", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// assertError wraps a message into an error so test rows can stay compact.
func assertError(msg string) error { return errString(msg) }

type errString string

func (e errString) Error() string { return string(e) }
