package handler

import (
	"context"
	"errors"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// ListDictionary handles GET /dictionaries/{type}.
//
// The endpoint is public (no auth). The service decides whether the requested
// type is known; an unknown type degrades to 404, not 500. Empty dictionaries
// are returned as 200 with an empty items array.
func (s *Server) ListDictionary(ctx context.Context, request api.ListDictionaryRequestObject) (api.ListDictionaryResponseObject, error) {
	pType := request.Type
	entries, err := s.dictionaryService.List(ctx, domain.DictionaryType(pType))
	if err != nil {
		if errors.Is(err, domain.ErrDictionaryUnknownType) {
			// Map unknown dictionary type onto the canonical 404 path so
			// respondError produces the same shape as any other not-found.
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	items := make([]api.DictionaryItem, len(entries))
	for i, e := range entries {
		items[i] = api.DictionaryItem{Code: e.Code, Name: e.Name, SortOrder: e.SortOrder}
	}
	return api.ListDictionary200JSONResponse{
		Data: api.ListDictionaryData{Type: string(pType), Items: items},
	}, nil
}
