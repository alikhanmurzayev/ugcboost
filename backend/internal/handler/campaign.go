package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

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
// lives in the upcoming GET /campaigns/{id} (chunk #4); echoing the whole
// row from create would just duplicate the read contract without value.
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

	campaign, err := s.campaignService.CreateCampaign(ctx, name, tmaURL)
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
