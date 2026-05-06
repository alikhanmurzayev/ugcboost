package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// CreatorRepoFactory enumerates the repos CreatorService needs. Kept narrow
// per backend-architecture.md § RepoFactory — the parallel
// CreatorApplicationRepoFactory carries a wider set for the approve flow but
// CreatorService only reads the creator-side tables.
type CreatorRepoFactory interface {
	NewCreatorRepo(db dbutil.DB) repository.CreatorRepo
	NewCreatorSocialRepo(db dbutil.DB) repository.CreatorSocialRepo
	NewCreatorCategoryRepo(db dbutil.DB) repository.CreatorCategoryRepo
	NewDictionaryRepo(db dbutil.DB) repository.DictionaryRepo
}

// CreatorService serves the creator-side aggregate read plus the paginated
// list. Pool stays the only DB handle the service receives; nothing it does
// mutates state, so a transaction would only add latency.
type CreatorService struct {
	pool        dbutil.Pool
	repoFactory CreatorRepoFactory
	logger      logger.Logger
}

// NewCreatorService wires the creator service.
func NewCreatorService(pool dbutil.Pool, repoFactory CreatorRepoFactory, log logger.Logger) *CreatorService {
	return &CreatorService{
		pool:        pool,
		repoFactory: repoFactory,
		logger:      log,
	}
}

// GetByID assembles the full creator aggregate by combining the creators row,
// the snapshot socials and categories, and dictionary hydration for the city
// and category names. All four reads run sequentially against the pool — none
// of them can cause partial writes so a transaction would only buy stale-read
// guarantees the moderation UI does not need.
//
// sql.ErrNoRows on the main lookup is translated into domain.ErrCreatorNotFound
// at the boundary so the handler maps it to 404 CREATOR_NOT_FOUND rather than
// the generic NOT_FOUND fallback. Errors from any subsequent read are wrapped
// so the originating layer is identifiable in logs.
//
// Deactivated dictionary codes (active=false on the underlying row) degrade
// gracefully: the code stays intact in the response and the localised name
// falls back to the code itself. This mirrors the CreatorApplicationService
// hydration behaviour so admins keep historical creator profiles readable
// even after a city or category is retired.
//
// Socials/categories arrive through ListByCreatorIDs with a single-element
// id array — the repos expose only the batched form (the list endpoint
// drives the same projection on multi-creator pages), so the GET aggregate
// reuses the same path with a one-element slice and pulls the matching entry
// out of the returned map.
func (s *CreatorService) GetByID(ctx context.Context, creatorID string) (*domain.CreatorAggregate, error) {
	creatorRow, err := s.repoFactory.NewCreatorRepo(s.pool).GetByID(ctx, creatorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCreatorNotFound
		}
		return nil, fmt.Errorf("get creator: %w", err)
	}

	socialsByID, err := s.repoFactory.NewCreatorSocialRepo(s.pool).ListByCreatorIDs(ctx, []string{creatorID})
	if err != nil {
		return nil, fmt.Errorf("list socials: %w", err)
	}
	socialRows := socialsByID[creatorID]

	categoriesByID, err := s.repoFactory.NewCreatorCategoryRepo(s.pool).ListByCreatorIDs(ctx, []string{creatorID})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	categoryCodes := categoriesByID[creatorID]

	dictRepo := s.repoFactory.NewDictionaryRepo(s.pool)

	cityRows, err := dictRepo.GetActiveByCodes(ctx, repository.TableCities, []string{creatorRow.CityCode})
	if err != nil {
		return nil, fmt.Errorf("lookup city: %w", err)
	}
	cityName := creatorRow.CityCode
	if len(cityRows) > 0 {
		cityName = cityRows[0].Name
	}

	categoryRows, err := dictRepo.GetActiveByCodes(ctx, repository.TableCategories, categoryCodes)
	if err != nil {
		return nil, fmt.Errorf("lookup categories: %w", err)
	}
	categoryNamesByCode := make(map[string]string, len(categoryRows))
	for _, row := range categoryRows {
		categoryNamesByCode[row.Code] = row.Name
	}

	categories := make([]domain.CreatorAggregateCategory, len(categoryCodes))
	for i, code := range categoryCodes {
		name := code
		if n, ok := categoryNamesByCode[code]; ok {
			name = n
		}
		categories[i] = domain.CreatorAggregateCategory{Code: code, Name: name}
	}

	socials := make([]domain.CreatorAggregateSocial, len(socialRows))
	for i, row := range socialRows {
		socials[i] = domain.CreatorAggregateSocial{
			ID:               row.ID,
			Platform:         row.Platform,
			Handle:           row.Handle,
			Verified:         row.Verified,
			Method:           row.Method,
			VerifiedByUserID: row.VerifiedByUserID,
			VerifiedAt:       row.VerifiedAt,
			CreatedAt:        row.CreatedAt,
		}
	}

	return &domain.CreatorAggregate{
		ID:                  creatorRow.ID,
		IIN:                 creatorRow.IIN,
		SourceApplicationID: creatorRow.SourceApplicationID,
		LastName:            creatorRow.LastName,
		FirstName:           creatorRow.FirstName,
		MiddleName:          creatorRow.MiddleName,
		BirthDate:           creatorRow.BirthDate,
		Phone:               creatorRow.Phone,
		CityCode:            creatorRow.CityCode,
		CityName:            cityName,
		Address:             creatorRow.Address,
		CategoryOtherText:   creatorRow.CategoryOtherText,
		TelegramUserID:      creatorRow.TelegramUserID,
		TelegramUsername:    creatorRow.TelegramUsername,
		TelegramFirstName:   creatorRow.TelegramFirstName,
		TelegramLastName:    creatorRow.TelegramLastName,
		Socials:             socials,
		Categories:          categories,
		CreatedAt:           creatorRow.CreatedAt,
		UpdatedAt:           creatorRow.UpdatedAt,
	}, nil
}

// List returns one page of approved creators matching the validated filter
// set. The handler has already enforced sort/order whitelists, page/perPage
// bounds and array sizes; this method trusts those invariants and focuses
// on (1) trimming the search query, (2) running the repo's page+count query,
// (3) batch-hydrating socials and categories so the read is N+1-free and
// (4) hydrating city + category dictionary names against the active
// dictionaries (deactivated codes degrade to the (code, code, 0) fallback,
// mirroring GetByID).
//
// The reads run against the pool directly — no transaction. Admin reads do
// not need cross-table consistency on the order of milliseconds; a brand-new
// creator appearing in the page query but not yet in the hydration query is
// acceptable (the missing rows degrade to empty arrays, not corrupt data).
func (s *CreatorService) List(ctx context.Context, in domain.CreatorListInput) (*domain.CreatorListPage, error) {
	params := creatorListInputToRepo(in)

	rows, total, err := s.repoFactory.NewCreatorRepo(s.pool).List(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list creators: %w", err)
	}
	if total == 0 || len(rows) == 0 {
		return &domain.CreatorListPage{
			Items:   nil,
			Total:   total,
			Page:    in.Page,
			PerPage: in.PerPage,
		}, nil
	}

	creatorIDs := make([]string, len(rows))
	for i, row := range rows {
		creatorIDs[i] = row.ID
	}

	socialsByID, err := s.repoFactory.NewCreatorSocialRepo(s.pool).ListByCreatorIDs(ctx, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("hydrate socials: %w", err)
	}
	categoriesByID, err := s.repoFactory.NewCreatorCategoryRepo(s.pool).ListByCreatorIDs(ctx, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("hydrate categories: %w", err)
	}

	items := make([]*domain.CreatorListItem, len(rows))
	for i, row := range rows {
		socialRows := socialsByID[row.ID]
		socials := make([]domain.CreatorListSocial, len(socialRows))
		for j, sr := range socialRows {
			socials[j] = domain.CreatorListSocial{
				Platform: sr.Platform,
				Handle:   sr.Handle,
			}
		}
		items[i] = &domain.CreatorListItem{
			ID:               row.ID,
			LastName:         row.LastName,
			FirstName:        row.FirstName,
			MiddleName:       row.MiddleName,
			IIN:              row.IIN,
			BirthDate:        row.BirthDate,
			Phone:            row.Phone,
			CityCode:         row.CityCode,
			Categories:       append([]string(nil), categoriesByID[row.ID]...),
			Socials:          socials,
			TelegramUsername: row.TelegramUsername,
			CreatedAt:        row.CreatedAt,
			UpdatedAt:        row.UpdatedAt,
		}
	}

	return &domain.CreatorListPage{
		Items:   items,
		Total:   total,
		Page:    in.Page,
		PerPage: in.PerPage,
	}, nil
}

// creatorListInputToRepo translates the validated handler input into the
// repo-shaped params struct. The handler is the single source of truth for
// validation (sort/order whitelisted, page/perPage bounded, search already
// trimmed by validateCreatorSearch); this mapping is a pure shape change.
func creatorListInputToRepo(in domain.CreatorListInput) repository.CreatorListParams {
	return repository.CreatorListParams{
		Cities:     in.Cities,
		Categories: in.Categories,
		DateFrom:   in.DateFrom,
		DateTo:     in.DateTo,
		AgeFrom:    in.AgeFrom,
		AgeTo:      in.AgeTo,
		Search:     in.Search,
		Sort:       in.Sort,
		Order:      in.Order,
		Page:       in.Page,
		PerPage:    in.PerPage,
	}
}
