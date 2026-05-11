package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

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
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
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

	participationsByCreator, campaignByID, err := s.loadCreatorParticipations(ctx, []string{creatorID})
	if err != nil {
		return nil, err
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

	participationRows := participationsByCreator[creatorID]
	campaigns := make([]domain.CreatorCampaignBrief, 0, len(participationRows))
	for _, p := range participationRows {
		c, ok := campaignByID[p.CampaignID]
		if !ok {
			continue
		}
		campaigns = append(campaigns, domain.CreatorCampaignBrief{
			ID:     c.ID,
			Name:   c.Name,
			Status: p.Status,
		})
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
		Campaigns:           campaigns,
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
	participationsByCreator, campaignByID, err := s.loadCreatorParticipations(ctx, creatorIDs)
	if err != nil {
		return nil, err
	}
	activeCountByCreator := make(map[string]int, len(creatorIDs))
	for creatorID, parts := range participationsByCreator {
		n := 0
		for _, p := range parts {
			if _, ok := campaignByID[p.CampaignID]; ok {
				n++
			}
		}
		activeCountByCreator[creatorID] = n
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
			ID:                   row.ID,
			LastName:             row.LastName,
			FirstName:            row.FirstName,
			MiddleName:           row.MiddleName,
			IIN:                  row.IIN,
			BirthDate:            row.BirthDate,
			Phone:                row.Phone,
			CityCode:             row.CityCode,
			Categories:           append([]string(nil), categoriesByID[row.ID]...),
			Socials:              socials,
			ActiveCampaignsCount: activeCountByCreator[row.ID],
			TelegramUsername:     row.TelegramUsername,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
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
		IDs:        in.IDs,
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

// loadCreatorParticipations batch-loads campaign_creators rows for the given
// creator ids and the non-deleted campaigns they reference. The
// `is_deleted = false` filter lives here rather than in SQL so the repo layer
// stays JOIN-free per spec. Returns:
//   - participations grouped by creator id, preserving the repo's
//     `created_at DESC, id DESC` ordering so the GET aggregate can stream
//     rows straight into CreatorCampaignBrief without re-sorting.
//   - lookup map of non-deleted campaign rows keyed by campaign id; consumers
//     filter participations by presence in this map.
//
// Empty input returns empty maps without hitting the database.
func (s *CreatorService) loadCreatorParticipations(
	ctx context.Context,
	creatorIDs []string,
) (map[string][]*repository.CampaignCreatorRow, map[string]*repository.CampaignRow, error) {
	if len(creatorIDs) == 0 {
		return map[string][]*repository.CampaignCreatorRow{}, map[string]*repository.CampaignRow{}, nil
	}

	participations, err := s.repoFactory.NewCampaignCreatorRepo(s.pool).ListByCreatorIDs(ctx, creatorIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list campaign participations: %w", err)
	}
	if len(participations) == 0 {
		return map[string][]*repository.CampaignCreatorRow{}, map[string]*repository.CampaignRow{}, nil
	}

	campaignIDSet := make(map[string]struct{}, len(participations))
	for _, p := range participations {
		campaignIDSet[p.CampaignID] = struct{}{}
	}
	campaignIDs := make([]string, 0, len(campaignIDSet))
	for id := range campaignIDSet {
		campaignIDs = append(campaignIDs, id)
	}
	// Sort the ids so ListByIDs receives a deterministic argument order. Map
	// iteration is randomized in Go and the unrelated test scaffolding would
	// otherwise need mock.MatchedBy for what is logically an exact-args
	// expectation.
	sort.Strings(campaignIDs)

	campaignRows, err := s.repoFactory.NewCampaignRepo(s.pool).ListByIDs(ctx, campaignIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list campaigns by ids: %w", err)
	}
	campaignByID := make(map[string]*repository.CampaignRow, len(campaignRows))
	for _, c := range campaignRows {
		if c.IsDeleted {
			continue
		}
		campaignByID[c.ID] = c
	}

	// Surface data-integrity drift: a campaign_creators row that points to a
	// campaign id absent from the campaigns table (neither soft-deleted nor
	// active) means the FK invariant is broken. Log without failing — the read
	// path keeps degrading gracefully (the orphan is hidden from the user) but
	// ops sees a signal before silent corruption spreads.
	for id := range campaignIDSet {
		if _, present := campaignByID[id]; present {
			continue
		}
		stillDeleted := false
		for _, c := range campaignRows {
			if c.ID == id && c.IsDeleted {
				stillDeleted = true
				break
			}
		}
		if stillDeleted {
			continue
		}
		s.logger.Warn(ctx, "campaign_creators references missing campaign", "campaign_id", id)
	}

	participationsByCreator := make(map[string][]*repository.CampaignCreatorRow, len(creatorIDs))
	for _, p := range participations {
		participationsByCreator[p.CreatorID] = append(participationsByCreator[p.CreatorID], p)
	}
	return participationsByCreator, campaignByID, nil
}
