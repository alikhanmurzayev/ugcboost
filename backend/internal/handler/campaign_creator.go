package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// campaignCreatorBatchMax mirrors the OpenAPI maxItems=200 constraint on
// AddCampaignCreatorsInput.creatorIds — kept as a server-side cap because
// oapi-codegen does not enforce schema limits at runtime.
const campaignCreatorBatchMax = 200

// AddCampaignCreators handles POST /campaigns/{id}/creators (admin-only).
//
// Authorisation runs first so non-admin callers receive 403 before any DB
// touch. The handler enforces the empty-batch and per-batch duplicate
// invariants before reaching the service — both surface as their own 422
// codes (CAMPAIGN_CREATOR_IDS_REQUIRED / CAMPAIGN_CREATOR_IDS_DUPLICATES).
// Strict-422 conflicts (creator does not exist, already in this campaign)
// arrive from the service via *ValidationError and are rendered by
// respondError without a per-call switch here.
func (s *Server) AddCampaignCreators(ctx context.Context, request api.AddCampaignCreatorsRequestObject) (api.AddCampaignCreatorsResponseObject, error) {
	if err := s.authzService.CanAddCampaignCreators(ctx); err != nil {
		return nil, err
	}
	if len(request.Body.CreatorIds) == 0 {
		return nil, domain.NewValidationError(
			domain.CodeCampaignCreatorIdsRequired,
			"Список креаторов обязателен. Передайте хотя бы один creatorId.",
		)
	}
	// oapi-codegen ignores schema-level maxItems at runtime; without this
	// cap an admin could submit thousands of creatorIds and pin a single
	// transaction (and connection-pool slot) for seconds.
	if len(request.Body.CreatorIds) > campaignCreatorBatchMax {
		return nil, domain.NewValidationError(
			domain.CodeCampaignCreatorIdsTooMany,
			"Слишком много креаторов в одном батче. Разбейте список на части по 200 и повторите.",
		)
	}

	seen := make(map[uuid.UUID]struct{}, len(request.Body.CreatorIds))
	creatorIDs := make([]string, 0, len(request.Body.CreatorIds))
	for _, id := range request.Body.CreatorIds {
		if _, dup := seen[id]; dup {
			return nil, domain.NewValidationError(
				domain.CodeCampaignCreatorIdsDuplicates,
				fmt.Sprintf("В списке креаторов дубликат: %s. Уберите повторяющиеся ID и повторите.", id),
			)
		}
		seen[id] = struct{}{}
		creatorIDs = append(creatorIDs, id.String())
	}

	items, err := s.campaignCreatorService.Add(ctx, request.Id.String(), creatorIDs)
	if err != nil {
		return nil, err
	}

	apiItems, err := domainCampaignCreatorsToAPI(items)
	if err != nil {
		return nil, err
	}
	resp := api.AddCampaignCreators201JSONResponse{}
	resp.Data.Items = apiItems
	return resp, nil
}

// RemoveCampaignCreator handles DELETE /campaigns/{id}/creators/{creatorId}
// (admin-only). The endpoint is empty on success (204); the service refuses
// to remove a creator whose row has reached status=agreed and surfaces
// ErrCampaignCreatorRemoveAfterAgreed as 422 via respondError's
// *ValidationError branch. Missing pair → 404 ErrCampaignCreatorNotFound.
func (s *Server) RemoveCampaignCreator(ctx context.Context, request api.RemoveCampaignCreatorRequestObject) (api.RemoveCampaignCreatorResponseObject, error) {
	if err := s.authzService.CanRemoveCampaignCreator(ctx); err != nil {
		return nil, err
	}
	if err := s.campaignCreatorService.Remove(ctx, request.Id.String(), request.CreatorId.String()); err != nil {
		return nil, err
	}
	return api.RemoveCampaignCreator204Response{}, nil
}

// ListCampaignCreators handles GET /campaigns/{id}/creators (admin-only). No
// pagination — chunk 10 spec returns the whole roster. Soft-deleted /
// missing campaigns surface as 404 from the service.
func (s *Server) ListCampaignCreators(ctx context.Context, request api.ListCampaignCreatorsRequestObject) (api.ListCampaignCreatorsResponseObject, error) {
	if err := s.authzService.CanListCampaignCreators(ctx); err != nil {
		return nil, err
	}
	items, err := s.campaignCreatorService.List(ctx, request.Id.String())
	if err != nil {
		return nil, err
	}
	apiItems, err := domainCampaignCreatorsToAPI(items)
	if err != nil {
		return nil, err
	}
	resp := api.ListCampaignCreators200JSONResponse{}
	resp.Data.Items = apiItems
	return resp, nil
}

// domainCampaignCreatorsToAPI maps a slice of domain rows into the strict-
// server projection. UUID parse failures only fire on a corrupted DB row
// (gen_random_uuid + FK to creators(id)) — the strict-server adapter renders
// the wrapped error as 500.
func domainCampaignCreatorsToAPI(items []*domain.CampaignCreator) ([]api.CampaignCreator, error) {
	out := make([]api.CampaignCreator, len(items))
	for i, cc := range items {
		mapped, err := domainCampaignCreatorToAPI(cc)
		if err != nil {
			return nil, err
		}
		out[i] = mapped
	}
	return out, nil
}

func domainCampaignCreatorToAPI(cc *domain.CampaignCreator) (api.CampaignCreator, error) {
	id, err := uuid.Parse(cc.ID)
	if err != nil {
		return api.CampaignCreator{}, fmt.Errorf("parse campaign_creator id %q: %w", cc.ID, err)
	}
	campaignID, err := uuid.Parse(cc.CampaignID)
	if err != nil {
		return api.CampaignCreator{}, fmt.Errorf("parse campaign id %q: %w", cc.CampaignID, err)
	}
	creatorID, err := uuid.Parse(cc.CreatorID)
	if err != nil {
		return api.CampaignCreator{}, fmt.Errorf("parse creator id %q: %w", cc.CreatorID, err)
	}
	return api.CampaignCreator{
		Id:            openapi_types.UUID(id),
		CampaignId:    openapi_types.UUID(campaignID),
		CreatorId:     openapi_types.UUID(creatorID),
		Status:        api.CampaignCreatorStatus(cc.Status),
		InvitedAt:     cc.InvitedAt,
		InvitedCount:  cc.InvitedCount,
		RemindedAt:    cc.RemindedAt,
		RemindedCount: cc.RemindedCount,
		DecidedAt:     cc.DecidedAt,
		CreatedAt:     cc.CreatedAt,
		UpdatedAt:     cc.UpdatedAt,
	}, nil
}
