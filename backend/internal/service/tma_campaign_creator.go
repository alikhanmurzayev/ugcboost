package service

import (
	"context"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// TmaCampaignCreatorRepoFactory creates the repos TmaCampaignCreatorService
// needs. The campaign-creator repo carries the row state machine; the audit
// repo records the agree/decline event in the same tx as the UPDATE so the
// audit row always matches what actually committed to campaign_creators.
type TmaCampaignCreatorRepoFactory interface {
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// TmaDecisionAuth is the post-authorisation context handed to ApplyDecision.
// It is produced by AuthzService.AuthorizeTMACampaignDecision after the
// caller's TMA initData has been validated and the (creator, campaign)
// link has been confirmed in DB. CampaignCreatorID is the row id the
// service updates inside the tx; CampaignID/CreatorID flow into the audit
// payload.
type TmaDecisionAuth struct {
	CampaignID        string
	CreatorID         string
	CampaignCreatorID string
}

// TmaCampaignCreatorService implements the creator-side decision flow on
// the TMA: agree / decline. The handler stays thin (regex-reject path
// param + AuthzService → here); business state machine + transition
// guarding lives in this service.
type TmaCampaignCreatorService struct {
	pool        dbutil.Pool
	repoFactory TmaCampaignCreatorRepoFactory
	logger      logger.Logger
}

// NewTmaCampaignCreatorService creates the service.
func NewTmaCampaignCreatorService(pool dbutil.Pool, repoFactory TmaCampaignCreatorRepoFactory, log logger.Logger) *TmaCampaignCreatorService {
	return &TmaCampaignCreatorService{pool: pool, repoFactory: repoFactory, logger: log}
}

// ApplyDecision flips the campaign_creators row to agreed / declined under
// `SELECT ... FOR UPDATE` row-lock. Idempotency is symmetric: a repeated
// agree from `agreed` (or decline from `declined`) returns 200 +
// AlreadyDecided=true without writing UPDATE or audit. Other transitions
// surface granular 422s. Audit (action `campaign_creator_agree` /
// `campaign_creator_decline`, actor_id NULL) is written in the same tx as
// the UPDATE — a rolled-back transition rolls back the audit row, so a
// committed audit always reflects the committed state.
func (s *TmaCampaignCreatorService) ApplyDecision(ctx context.Context, auth TmaDecisionAuth, decision domain.CampaignCreatorDecision) (domain.CampaignCreatorDecisionResult, error) {
	var result domain.CampaignCreatorDecisionResult
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		ccRepo := s.repoFactory.NewCampaignCreatorRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		cc, err := ccRepo.GetByIDForUpdate(ctx, auth.CampaignCreatorID)
		if err != nil {
			return fmt.Errorf("lock campaign_creator: %w", err)
		}

		newStatus, transition, err := decideTransition(cc.Status, decision)
		if err != nil {
			return err
		}
		if !transition {
			result = domain.CampaignCreatorDecisionResult{Status: cc.Status, AlreadyDecided: true}
			return nil
		}

		updated, err := ccRepo.ApplyDecision(ctx, auth.CampaignCreatorID, newStatus)
		if err != nil {
			return fmt.Errorf("apply decision: %w", err)
		}

		action := AuditActionCampaignCreatorAgree
		if decision == domain.CampaignCreatorDecisionDecline {
			action = AuditActionCampaignCreatorDecline
		}
		payload := map[string]string{
			"campaign_id": auth.CampaignID,
			"creator_id":  auth.CreatorID,
		}
		if err := writeAudit(ctx, auditRepo,
			action, AuditEntityTypeCampaignCreator, auth.CampaignCreatorID,
			nil, payload); err != nil {
			return fmt.Errorf("audit decision: %w", err)
		}

		result = domain.CampaignCreatorDecisionResult{Status: updated.Status, AlreadyDecided: false}
		return nil
	})
	if err != nil {
		return domain.CampaignCreatorDecisionResult{}, err
	}
	s.logger.Info(ctx, "tma decision applied",
		"campaign_creator_id", auth.CampaignCreatorID,
		"decision", string(decision),
		"already_decided", result.AlreadyDecided)
	return result, nil
}

// decideTransition encodes the (current, decision) → (new, transition?)
// state machine. Returns transition=false for idempotent no-ops (already
// in the requested terminal state). Returns a granular ValidationError for
// incompatible transitions. The caller (ApplyDecision) treats both ok
// branches the same way except for the audit/UPDATE skip.
func decideTransition(current string, decision domain.CampaignCreatorDecision) (newStatus string, transition bool, err error) {
	switch current {
	case domain.CampaignCreatorStatusInvited:
		switch decision {
		case domain.CampaignCreatorDecisionAgree:
			return domain.CampaignCreatorStatusAgreed, true, nil
		case domain.CampaignCreatorDecisionDecline:
			return domain.CampaignCreatorStatusDeclined, true, nil
		}
	case domain.CampaignCreatorStatusAgreed:
		if decision == domain.CampaignCreatorDecisionAgree {
			return domain.CampaignCreatorStatusAgreed, false, nil
		}
		return "", false, domain.ErrCampaignCreatorAlreadyAgreed
	case domain.CampaignCreatorStatusDeclined:
		if decision == domain.CampaignCreatorDecisionDecline {
			return domain.CampaignCreatorStatusDeclined, false, nil
		}
		return "", false, domain.ErrCampaignCreatorDeclinedNeedReinvite
	case domain.CampaignCreatorStatusPlanned:
		return "", false, domain.ErrCampaignCreatorNotInvited
	}
	return "", false, fmt.Errorf("unexpected campaign_creator status %q", current)
}
