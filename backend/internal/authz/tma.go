package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// AuthorizeTMACampaignDecision is the authz pre-pass for the TMA agree /
// decline endpoints. It runs after the `tma_initdata` middleware has
// (a) verified the HMAC and stamped telegram_user_id into ctx, and
// (b) optionally resolved the matching creator row (creator_id + role).
//
// Three gates fire in order, all anti-fingerprint to the 401-style of
// the middleware itself:
//
//  1. Role missing in ctx (creator not registered) — 403 ErrTMAForbidden.
//  2. Campaign not found by secret_token (or soft-deleted) — 404
//     ErrCampaignNotFound. The repo filters is_deleted=false.
//  3. Creator not in this campaign — 403 ErrTMAForbidden. Same code as
//     gate 1 by design — anti-fingerprint between "creator does not exist"
//     and "creator exists but is not invited".
//
// On success returns the post-authz tuple the service uses to write its
// audit row and to flip the campaign_creators state machine.
func (a *AuthzService) AuthorizeTMACampaignDecision(ctx context.Context, secretToken string) (TMACampaignDecisionAuth, error) {
	if middleware.RoleFromContext(ctx) != api.Creator {
		return TMACampaignDecisionAuth{}, domain.ErrTMAForbidden
	}
	creatorID := middleware.CreatorIDFromContext(ctx)
	if creatorID == "" {
		return TMACampaignDecisionAuth{}, domain.ErrTMAForbidden
	}

	campaign, err := a.repoFactory.NewCampaignRepo(a.pool).GetBySecretToken(ctx, secretToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TMACampaignDecisionAuth{}, domain.ErrCampaignNotFound
		}
		return TMACampaignDecisionAuth{}, fmt.Errorf("authz lookup campaign: %w", err)
	}

	cc, err := a.repoFactory.NewCampaignCreatorRepo(a.pool).GetByCampaignAndCreator(ctx, campaign.ID, creatorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TMACampaignDecisionAuth{}, domain.ErrTMAForbidden
		}
		return TMACampaignDecisionAuth{}, fmt.Errorf("authz lookup campaign_creator: %w", err)
	}

	return TMACampaignDecisionAuth{
		CreatorID:         creatorID,
		CampaignID:        campaign.ID,
		CampaignCreatorID: cc.ID,
		CurrentStatus:     cc.Status,
	}, nil
}

// TMACampaignDecisionAuth is the post-authorisation tuple consumed by
// TmaCampaignCreatorService.ApplyDecision. CurrentStatus is informational
// — the service re-reads it under FOR UPDATE inside its own tx for the
// state-machine transition guard.
type TMACampaignDecisionAuth struct {
	CreatorID         string
	CampaignID        string
	CampaignCreatorID string
	CurrentStatus     string
}
