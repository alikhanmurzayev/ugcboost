package service

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// DictionaryRepoFactory enumerates the repos the dictionary service needs.
// It mirrors a constructor on repository.RepoFactory.
type DictionaryRepoFactory interface {
	NewDictionaryRepo(db dbutil.DB) repository.DictionaryRepo
}

// dictionaryTables maps a public dictionary type onto the underlying DB
// table. Adding a new dictionary = new entry here + new DB table + new
// constant in domain.DictionaryTypeValues. No string literals — every table
// name comes from the repository package's exported constants.
var dictionaryTables = map[domain.DictionaryType]string{
	domain.DictionaryTypeCategories: repository.TableCategories,
	domain.DictionaryTypeCities:     repository.TableCities,
}

// DictionaryService is a thin read-only layer that serves the public
// dictionaries used by the landing page (categories, cities). All entries
// are seeded via migrations; runtime writes are not in scope.
type DictionaryService struct {
	db          dbutil.DB
	repoFactory DictionaryRepoFactory
	logger      logger.Logger
}

// NewDictionaryService wires the service with its dependencies.
func NewDictionaryService(db dbutil.DB, repoFactory DictionaryRepoFactory, log logger.Logger) *DictionaryService {
	return &DictionaryService{db: db, repoFactory: repoFactory, logger: log}
}

// List returns all active entries of the given dictionary, mapped to the
// transport-agnostic domain.DictionaryEntry. Unknown dictionary types yield
// domain.ErrDictionaryUnknownType so the handler can answer 404.
func (s *DictionaryService) List(ctx context.Context, t domain.DictionaryType) ([]domain.DictionaryEntry, error) {
	table, ok := dictionaryTables[t]
	if !ok {
		return nil, domain.ErrDictionaryUnknownType
	}
	rows, err := s.repoFactory.NewDictionaryRepo(s.db).ListActive(ctx, table)
	if err != nil {
		return nil, err
	}
	out := make([]domain.DictionaryEntry, len(rows))
	for i, r := range rows {
		out[i] = domain.DictionaryEntry{Code: r.Code, Name: r.Name, SortOrder: r.SortOrder}
	}
	return out, nil
}
