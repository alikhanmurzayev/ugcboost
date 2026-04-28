package handler

import (
	"errors"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// ListDictionary handles GET /dictionaries/{type}.
//
// The endpoint is public (no auth). The service decides whether the requested
// type is known; an unknown type degrades to 404, not 500. Empty dictionaries
// are returned as 200 with an empty items array.
func (s *Server) ListDictionary(w http.ResponseWriter, r *http.Request, pType api.ListDictionaryParamsType) {
	entries, err := s.dictionaryService.List(r.Context(), domain.DictionaryType(pType))
	if err != nil {
		if errors.Is(err, domain.ErrDictionaryUnknownType) {
			// Map unknown dictionary type onto the canonical 404 path so
			// respondError produces the same shape as any other not-found.
			respondError(w, r, domain.ErrNotFound, s.logger)
			return
		}
		respondError(w, r, err, s.logger)
		return
	}
	items := make([]api.DictionaryEntry, len(entries))
	for i, e := range entries {
		items[i] = api.DictionaryEntry{Code: e.Code, Name: e.Name, SortOrder: e.SortOrder}
	}
	respondJSON(w, r, http.StatusOK, api.DictionaryListResult{
		Data: api.ListDictionaryData{Type: string(pType), Items: items},
	}, s.logger)
}
