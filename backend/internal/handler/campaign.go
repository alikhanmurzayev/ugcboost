package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// CreateCampaign handles POST /campaigns (admin-only).
//
// Authorisation runs first so non-admin callers receive 403 before any DB
// touch. After authz, name and tmaUrl pass through the domain validators
// which trim AND check granular CodeCampaign* — empty / >255-name / empty
// / >2048-url each surface as their own 422 code. The 23505 race on
// campaigns_name_active_unique is translated by the repo into
// domain.ErrCampaignNameTaken (a *BusinessError) and rendered as 409
// CAMPAIGN_NAME_TAKEN by respondError's generic *BusinessError branch.
//
// Response carries only the freshly created id — the full read aggregate
// lives in GET /campaigns/{id}; echoing the whole row from create would
// just duplicate the read contract without value.
func (s *Server) CreateCampaign(ctx context.Context, request api.CreateCampaignRequestObject) (api.CreateCampaignResponseObject, error) {
	if err := s.authzService.CanCreateCampaign(ctx); err != nil {
		return nil, err
	}

	name, err := domain.ValidateCampaignName(request.Body.Name)
	if err != nil {
		return nil, err
	}
	tmaURL, err := domain.ValidateCampaignTmaURL(request.Body.TmaUrl)
	if err != nil {
		return nil, err
	}

	campaign, err := s.campaignService.CreateCampaign(ctx, domain.CampaignInput{Name: name, TmaURL: tmaURL})
	if err != nil {
		return nil, err
	}

	// Defensive parse — campaign.ID is stamped by gen_random_uuid() at INSERT,
	// so this branch only fires on a corrupted DB row, not user input.
	id, err := uuid.Parse(campaign.ID)
	if err != nil {
		return nil, fmt.Errorf("parse campaign id %q: %w", campaign.ID, err)
	}
	return api.CreateCampaign201JSONResponse{Data: api.CampaignCreatedData{Id: id}}, nil
}

// GetCampaign handles GET /campaigns/{id} (admin-only).
//
// Authorisation runs first so non-admin callers see 403 before any DB read;
// that keeps the response timing identical regardless of whether the campaign
// exists. ErrCampaignNotFound from the service is mapped to 404
// CAMPAIGN_NOT_FOUND by respondError. Soft-deleted campaigns are returned as
// well — the live/deleted split lives in the upcoming list endpoint, not
// here.
func (s *Server) GetCampaign(ctx context.Context, request api.GetCampaignRequestObject) (api.GetCampaignResponseObject, error) {
	if err := s.authzService.CanGetCampaign(ctx); err != nil {
		return nil, err
	}

	campaign, err := s.campaignService.GetByID(ctx, request.Id.String())
	if err != nil {
		return nil, err
	}

	data, err := domainCampaignToAPI(campaign)
	if err != nil {
		return nil, err
	}
	return api.GetCampaign200JSONResponse{Data: data}, nil
}

// UpdateCampaign handles PATCH /campaigns/{id} (admin-only); success returns 204.
func (s *Server) UpdateCampaign(ctx context.Context, request api.UpdateCampaignRequestObject) (api.UpdateCampaignResponseObject, error) {
	if err := s.authzService.CanUpdateCampaign(ctx); err != nil {
		return nil, err
	}

	name, err := domain.ValidateCampaignName(request.Body.Name)
	if err != nil {
		return nil, err
	}
	tmaURL, err := domain.ValidateCampaignTmaURL(request.Body.TmaUrl)
	if err != nil {
		return nil, err
	}

	if err := s.campaignService.UpdateCampaign(ctx, request.Id.String(),
		domain.CampaignInput{Name: name, TmaURL: tmaURL}); err != nil {
		return nil, err
	}
	return api.UpdateCampaign204Response{}, nil
}

// ListCampaigns handles GET /campaigns (admin-only).
func (s *Server) ListCampaigns(ctx context.Context, request api.ListCampaignsRequestObject) (api.ListCampaignsResponseObject, error) {
	if err := s.authzService.CanListCampaigns(ctx); err != nil {
		return nil, err
	}
	params := request.Params

	if !params.Sort.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение sort. Допустимы: %s",
				strings.Join(domain.CampaignListSortFieldValues, ", ")))
	}
	if !params.Order.Valid() {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Неподдерживаемое значение order. Допустимы: %s",
				strings.Join(domain.SortOrderValues, ", ")))
	}
	if params.Page < domain.CampaignListPageMin || params.Page > domain.CampaignListPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр page должен быть в диапазоне %d..%d",
				domain.CampaignListPageMin, domain.CampaignListPageMax))
	}
	if params.PerPage < domain.CampaignListPerPageMin || params.PerPage > domain.CampaignListPerPageMax {
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("Параметр perPage должен быть в диапазоне %d..%d",
				domain.CampaignListPerPageMin, domain.CampaignListPerPageMax))
	}

	search := ""
	if params.Search != nil {
		trimmed := strings.TrimSpace(*params.Search)
		if len([]rune(trimmed)) > domain.CampaignListSearchMaxLen {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("Параметр search не должен превышать %d символов",
					domain.CampaignListSearchMaxLen))
		}
		search = trimmed
	}

	in := domain.CampaignListInput{
		Search:    search,
		IsDeleted: params.IsDeleted,
		Sort:      string(params.Sort),
		Order:     string(params.Order),
		Page:      params.Page,
		PerPage:   params.PerPage,
	}

	page, err := s.campaignService.List(ctx, in)
	if err != nil {
		return nil, err
	}

	items := make([]api.Campaign, len(page.Items))
	for i, c := range page.Items {
		mapped, err := domainCampaignToAPI(c)
		if err != nil {
			return nil, err
		}
		items[i] = mapped
	}
	return api.ListCampaigns200JSONResponse{
		Data: api.CampaignsListData{
			Items:   items,
			Total:   page.Total,
			Page:    page.Page,
			PerPage: page.PerPage,
		},
	}, nil
}

// domainCampaignToAPI maps the domain campaign onto its strict-server
// counterpart. UUID parse failure on the stored id surfaces as a wrapped
// error so the strict-server adapter renders 500 — the row is populated by
// gen_random_uuid() so a parse failure means real corruption, not user input.
func domainCampaignToAPI(c *domain.Campaign) (api.Campaign, error) {
	id, err := uuid.Parse(c.ID)
	if err != nil {
		return api.Campaign{}, fmt.Errorf("parse campaign id %q: %w", c.ID, err)
	}
	return api.Campaign{
		Id:        openapi_types.UUID(id),
		Name:      c.Name,
		TmaUrl:    c.TmaURL,
		IsDeleted: c.IsDeleted,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}, nil
}
