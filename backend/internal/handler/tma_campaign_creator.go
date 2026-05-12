package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// TmaAgree handles POST /tma/campaigns/{secret_token}/agree.
//
// Auth pipeline:
//  1. The `tma_initdata` middleware (registered on /tma/* in main.go) has
//     already verified the HMAC and stamped telegram_user_id (+ creator_id /
//     role when the creator exists) into ctx; failures are 401 anti-fingerprint
//     before this method runs.
//  2. The handler itself rejects malformed `secretToken` path parameters
//     with 404 anti-fingerprint *before* any DB lookup. The strict regex
//     match closes the suffix-attack surface that would otherwise reach
//     `WHERE secret_token = $1` with a one-character probe.
//  3. AuthzService.AuthorizeTMACampaignDecision runs the role/creator/
//     campaign/membership gates and returns the post-authz tuple.
//  4. Service.ApplyDecision flips the row + writes audit in one tx.
func (s *Server) TmaAgree(ctx context.Context, request api.TmaAgreeRequestObject) (api.TmaAgreeResponseObject, error) {
	if !domain.SecretTokenRegex().MatchString(string(request.SecretToken)) {
		return nil, domain.ErrCampaignNotFound
	}
	auth, err := s.authzService.AuthorizeTMACampaignDecision(ctx, string(request.SecretToken))
	if err != nil {
		return nil, err
	}
	result, err := s.tmaCampaignCreatorService.ApplyDecision(ctx,
		service.TmaDecisionAuth{
			CampaignID:        auth.CampaignID,
			CreatorID:         auth.CreatorID,
			CampaignCreatorID: auth.CampaignCreatorID,
		},
		domain.CampaignCreatorDecisionAgree)
	if err != nil {
		return nil, err
	}
	return api.TmaAgree200JSONResponse{
		Status:         api.CampaignCreatorStatus(result.Status),
		AlreadyDecided: result.AlreadyDecided,
	}, nil
}

// TmaGetParticipation handles GET /tma/campaigns/{secret_token}/participation.
// Read-only counterpart to TmaAgree/TmaDecline used by the TMA brief page to
// decide whether to render the agree/decline buttons (visible only when the
// row is in `invited`). Reuses the same authz pre-pass — the resulting
// TMACampaignDecisionAuth already carries CurrentStatus, so no service / repo
// hop is required and no audit row is written.
func (s *Server) TmaGetParticipation(ctx context.Context, request api.TmaGetParticipationRequestObject) (api.TmaGetParticipationResponseObject, error) {
	if !domain.SecretTokenRegex().MatchString(string(request.SecretToken)) {
		return nil, domain.ErrCampaignNotFound
	}
	auth, err := s.authzService.AuthorizeTMACampaignDecision(ctx, string(request.SecretToken))
	if err != nil {
		return nil, err
	}
	return api.TmaGetParticipation200JSONResponse{
		Status: api.CampaignCreatorStatus(auth.CurrentStatus),
	}, nil
}

// TmaDecline handles POST /tma/campaigns/{secret_token}/decline. Symmetric
// counterpart to TmaAgree — same regex / authz / service guards, flips the
// row to declined.
func (s *Server) TmaDecline(ctx context.Context, request api.TmaDeclineRequestObject) (api.TmaDeclineResponseObject, error) {
	if !domain.SecretTokenRegex().MatchString(string(request.SecretToken)) {
		return nil, domain.ErrCampaignNotFound
	}
	auth, err := s.authzService.AuthorizeTMACampaignDecision(ctx, string(request.SecretToken))
	if err != nil {
		return nil, err
	}
	result, err := s.tmaCampaignCreatorService.ApplyDecision(ctx,
		service.TmaDecisionAuth{
			CampaignID:        auth.CampaignID,
			CreatorID:         auth.CreatorID,
			CampaignCreatorID: auth.CampaignCreatorID,
		},
		domain.CampaignCreatorDecisionDecline)
	if err != nil {
		return nil, err
	}
	return api.TmaDecline200JSONResponse{
		Status:         api.CampaignCreatorStatus(result.Status),
		AlreadyDecided: result.AlreadyDecided,
	}, nil
}
