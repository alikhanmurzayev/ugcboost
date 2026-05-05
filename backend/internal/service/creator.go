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

// CreatorService serves the creator-side aggregate read. Pool stays the only
// DB handle the service receives; nothing it does mutates state, so a
// transaction would only add latency.
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
func (s *CreatorService) GetByID(ctx context.Context, creatorID string) (*domain.CreatorAggregate, error) {
	creatorRow, err := s.repoFactory.NewCreatorRepo(s.pool).GetByID(ctx, creatorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCreatorNotFound
		}
		return nil, fmt.Errorf("get creator: %w", err)
	}

	socialRows, err := s.repoFactory.NewCreatorSocialRepo(s.pool).ListByCreatorID(ctx, creatorID)
	if err != nil {
		return nil, fmt.Errorf("list socials: %w", err)
	}

	categoryCodes, err := s.repoFactory.NewCreatorCategoryRepo(s.pool).ListByCreatorID(ctx, creatorID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

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
