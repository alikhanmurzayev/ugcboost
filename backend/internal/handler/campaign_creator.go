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
	creatorIDs, err := validateCampaignCreatorBatch(request.Body.CreatorIds)
	if err != nil {
		return nil, err
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

// NotifyCampaignCreators handles POST /campaigns/{id}/notify (admin-only).
// The handler enforces the same shape invariants as AddCampaignCreators
// (non-empty / ≤200 / no duplicates) before reaching the service. Strict-
// 422 batch validation (creator not in campaign / wrong status) and 200
// partial-success delivery (with `undelivered`) come back from the service;
// respondError handles the dedicated CAMPAIGN_CREATOR_BATCH_INVALID schema.
func (s *Server) NotifyCampaignCreators(ctx context.Context, request api.NotifyCampaignCreatorsRequestObject) (api.NotifyCampaignCreatorsResponseObject, error) {
	if err := s.authzService.CanNotifyCampaignCreators(ctx); err != nil {
		return nil, err
	}
	creatorIDs, err := validateCampaignCreatorBatch(request.Body.CreatorIds)
	if err != nil {
		return nil, err
	}
	undelivered, err := s.campaignCreatorService.Notify(ctx, request.Id.String(), creatorIDs)
	if err != nil {
		return nil, err
	}
	resp := api.NotifyCampaignCreators200JSONResponse{}
	resp.Data.Undelivered = s.domainNotifyFailuresToAPI(ctx, undelivered)
	return resp, nil
}

// RemindCampaignCreatorsInvitation handles POST /campaigns/{id}/remind-
// invitation (admin-only). Symmetric to NotifyCampaignCreators in shape
// validation, partial-success and error handling; the only difference is
// the service method (RemindInvitation) and the implied source-status
// guard (`invited`).
func (s *Server) RemindCampaignCreatorsInvitation(ctx context.Context, request api.RemindCampaignCreatorsInvitationRequestObject) (api.RemindCampaignCreatorsInvitationResponseObject, error) {
	if err := s.authzService.CanRemindCampaignCreators(ctx); err != nil {
		return nil, err
	}
	creatorIDs, err := validateCampaignCreatorBatch(request.Body.CreatorIds)
	if err != nil {
		return nil, err
	}
	undelivered, err := s.campaignCreatorService.RemindInvitation(ctx, request.Id.String(), creatorIDs)
	if err != nil {
		return nil, err
	}
	resp := api.RemindCampaignCreatorsInvitation200JSONResponse{}
	resp.Data.Undelivered = s.domainNotifyFailuresToAPI(ctx, undelivered)
	return resp, nil
}

// validateCampaignCreatorBatch enforces the empty / ≤200 / no-duplicates
// guard shared by AddCampaignCreators, NotifyCampaignCreators and
// RemindCampaignCreatorsInvitation. Returns the deduplicated list of
// stringified UUIDs ready for the service layer.
func validateCampaignCreatorBatch(in []openapi_types.UUID) ([]string, error) {
	if len(in) == 0 {
		return nil, domain.NewValidationError(
			domain.CodeCampaignCreatorIdsRequired,
			"Список креаторов обязателен. Передайте хотя бы один creatorId.",
		)
	}
	if len(in) > campaignCreatorBatchMax {
		return nil, domain.NewValidationError(
			domain.CodeCampaignCreatorIdsTooMany,
			"Слишком много креаторов в одном батче. Разбейте список на части по 200 и повторите.",
		)
	}
	seen := make(map[uuid.UUID]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
		if _, dup := seen[id]; dup {
			return nil, domain.NewValidationError(
				domain.CodeCampaignCreatorIdsDuplicates,
				fmt.Sprintf("В списке креаторов дубликат: %s. Уберите повторяющиеся ID и повторите.", id),
			)
		}
		seen[id] = struct{}{}
		out = append(out, id.String())
	}
	return out, nil
}

// domainNotifyFailuresToAPI maps the per-creator failures from the service
// to the OpenAPI projection used in the 200 response. UUID parse failures
// can only fire on a corrupted DB row (creator ids come from gen_random_uuid)
// — we log so a regression surfaces in stdout and fall back to uuid.Nil so
// the response shape is still valid.
func (s *Server) domainNotifyFailuresToAPI(ctx context.Context, failures []domain.NotifyFailure) []api.CampaignNotifyUndelivered {
	out := make([]api.CampaignNotifyUndelivered, 0, len(failures))
	for _, f := range failures {
		parsed, err := uuid.Parse(f.CreatorID)
		if err != nil {
			s.logger.Error(ctx, "domainNotifyFailuresToAPI: invalid creator UUID",
				"error", err, "creator_id", f.CreatorID)
		}
		out = append(out, api.CampaignNotifyUndelivered{
			CreatorId: openapi_types.UUID(parsed),
			Reason:    api.CampaignNotifyUndeliveredReason(f.Reason),
		})
	}
	return out
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
