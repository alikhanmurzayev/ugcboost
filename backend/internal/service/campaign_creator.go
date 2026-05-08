package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// CampaignCreatorRepoFactory creates the repositories CampaignCreatorService
// needs. The campaign repo is here for the soft-delete pre-fetch and to
// resolve tma_url for chunk-12 outbound messages; the creator repo resolves
// telegram_user_id chat ids for the same flow.
type CampaignCreatorRepoFactory interface {
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewCreatorRepo(db dbutil.DB) repository.CreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CampaignInviteNotifier abstracts the synchronous Telegram send used by the
// chunk-12 notify / remind-invitation flow. *telegram.Notifier satisfies it
// directly; tests inject a spy that records args and forces controlled
// errors for partial-success scenarios.
type CampaignInviteNotifier interface {
	SendCampaignInvite(ctx context.Context, chatID int64, text, tmaURL string) error
}

// CampaignCreatorService owns the admin-only attachment lifecycle: chunk-10
// batch add (→ planned), single remove (forbidden once agreed), no-pagination
// list, and chunk-12 notify (→ invited) / remind-invitation (counter bump).
// TMA-side agree / decline land in chunk 14.
type CampaignCreatorService struct {
	pool        dbutil.Pool
	repoFactory CampaignCreatorRepoFactory
	notifier    CampaignInviteNotifier
	logger      logger.Logger
}

// NewCampaignCreatorService creates a new CampaignCreatorService.
func NewCampaignCreatorService(
	pool dbutil.Pool,
	repoFactory CampaignCreatorRepoFactory,
	notifier CampaignInviteNotifier,
	log logger.Logger,
) *CampaignCreatorService {
	return &CampaignCreatorService{
		pool:        pool,
		repoFactory: repoFactory,
		notifier:    notifier,
		logger:      log,
	}
}

// Add inserts one campaign_creators row per creatorId in initial state
// `planned` and writes one audit-row per creator in the same transaction.
// The pre-fetch enforces "soft-deleted campaign = 404" before opening the
// transaction so a doomed batch never burns a tx; the matching FK race
// inside the loop is still translated into ErrCampaignNotFound to cover the
// soft-delete-during-batch corner case. Any failure rolls back the whole
// batch — strict-422 contract for the endpoint.
func (s *CampaignCreatorService) Add(ctx context.Context, campaignID string, creatorIDs []string) ([]*domain.CampaignCreator, error) {
	if err := s.assertCampaignActive(ctx, campaignID); err != nil {
		return nil, err
	}

	// Sort to enforce a deterministic per-row lock order across concurrent
	// admins. Without it, two batches like [A,B] and [B,A] can grab unique-
	// index locks in opposite directions and deadlock (PG 40P01) — Postgres
	// kills one side and the admin sees a 500 on legitimate input.
	creatorIDs = slices.Clone(creatorIDs)
	slices.Sort(creatorIDs)

	var result []*domain.CampaignCreator
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		ccRepo := s.repoFactory.NewCampaignCreatorRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)
		result = make([]*domain.CampaignCreator, 0, len(creatorIDs))

		for _, creatorID := range creatorIDs {
			row, err := ccRepo.Add(ctx, campaignID, creatorID, domain.CampaignCreatorStatusPlanned)
			if err != nil {
				return err
			}
			cc := campaignCreatorRowToDomain(row)
			if err := writeAudit(ctx, auditRepo,
				AuditActionCampaignCreatorAdd, AuditEntityTypeCampaignCreator, cc.ID,
				nil, cc); err != nil {
				return err
			}
			result = append(result, cc)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Debug, not Info: ApproveApplication invokes Add once per attached campaign,
	// so a 20-campaign approve floods INFO with 20 near-identical lines while the
	// audit_logs row inside the tx already authoritatively records each add.
	s.logger.Debug(ctx, "campaign creators added",
		"campaign_id", campaignID, "count", len(result))
	return result, nil
}

// Remove hard-deletes the (campaignId, creatorId) row and writes the
// matching audit-row in the same transaction. Pre-fetch enforces "soft-
// deleted campaign = 404"; the row read inside WithTx fills the audit
// snapshot and powers the agreed-status guard (LBYL). Once the row is in
// status=agreed it stays for the downstream TrustMe flow — Remove returns
// ErrCampaignCreatorRemoveAfterAgreed.
func (s *CampaignCreatorService) Remove(ctx context.Context, campaignID, creatorID string) error {
	if err := s.assertCampaignActive(ctx, campaignID); err != nil {
		return err
	}

	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		ccRepo := s.repoFactory.NewCampaignCreatorRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := ccRepo.GetByCampaignAndCreator(ctx, campaignID, creatorID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignCreatorNotFound
			}
			return fmt.Errorf("get campaign creator: %w", err)
		}
		if row.Status == domain.CampaignCreatorStatusAgreed {
			return domain.ErrCampaignCreatorRemoveAfterAgreed
		}
		oldCC := campaignCreatorRowToDomain(row)

		if err := ccRepo.DeleteByID(ctx, row.ID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignCreatorNotFound
			}
			return fmt.Errorf("delete campaign creator: %w", err)
		}

		return writeAudit(ctx, auditRepo,
			AuditActionCampaignCreatorRemove, AuditEntityTypeCampaignCreator, oldCC.ID,
			oldCC, nil)
	})
	if err != nil {
		return err
	}
	s.logger.Info(ctx, "campaign creator removed",
		"campaign_id", campaignID, "creator_id", creatorID)
	return nil
}

// List returns every creator attached to the campaign ordered by created_at
// ASC, id ASC. The read runs against the pool — no transaction, no audit,
// no success log (read paths stay quiet per security.md). The same soft-
// delete gate applies as on the mutate endpoints.
func (s *CampaignCreatorService) List(ctx context.Context, campaignID string) ([]*domain.CampaignCreator, error) {
	if err := s.assertCampaignActive(ctx, campaignID); err != nil {
		return nil, err
	}

	rows, err := s.repoFactory.NewCampaignCreatorRepo(s.pool).ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list campaign creators: %w", err)
	}
	items := make([]*domain.CampaignCreator, len(rows))
	for i, row := range rows {
		items[i] = campaignCreatorRowToDomain(row)
	}
	return items, nil
}

// batchOp distinguishes the two chunk-12 flows that share the
// dispatchBatch pipeline. Each branch carries its allowed source statuses,
// the audit action label, the bot copy, and the repo method that advances
// the campaign_creators row — all stored in batchOpSpecs to avoid
// unreachable default arms in the dispatch path.
type batchOp string

const (
	batchOpNotify           batchOp = "notify"
	batchOpRemindInvitation batchOp = "remind_invitation"
)

// batchOpSpec captures every branch-specific difference between Notify and
// RemindInvitation in one struct so dispatchBatch can look them up instead
// of switching on op at every step.
type batchOpSpec struct {
	allowedStatuses map[string]bool
	auditAction     string
	text            string
	apply           func(repo repository.CampaignCreatorRepo, ctx context.Context, id string) (*repository.CampaignCreatorRow, error)
}

var batchOpSpecs = map[batchOp]batchOpSpec{
	batchOpNotify: {
		allowedStatuses: map[string]bool{
			domain.CampaignCreatorStatusPlanned:  true,
			domain.CampaignCreatorStatusDeclined: true,
		},
		auditAction: AuditActionCampaignCreatorInvite,
		text:        telegram.CampaignInviteText(),
		apply: func(r repository.CampaignCreatorRepo, ctx context.Context, id string) (*repository.CampaignCreatorRow, error) {
			return r.ApplyInvite(ctx, id)
		},
	},
	batchOpRemindInvitation: {
		allowedStatuses: map[string]bool{
			domain.CampaignCreatorStatusInvited: true,
		},
		auditAction: AuditActionCampaignCreatorRemind,
		text:        telegram.CampaignRemindInvitationText(),
		apply: func(r repository.CampaignCreatorRepo, ctx context.Context, id string) (*repository.CampaignCreatorRow, error) {
			return r.ApplyRemind(ctx, id)
		},
	},
}

// Notify advances every creatorId in the batch to status=invited via a
// per-creator transaction guarded by partial-success delivery semantics.
// Whole-batch validation runs first against the read-only pool; any creator
// not attached to the campaign or sitting in an incompatible status
// (`invited`/`agreed`) collapses the request into a strict-422 with
// CAMPAIGN_CREATOR_BATCH_INVALID — no message is sent, the database is
// untouched. Past validation, each creator is dispatched independently:
// successful Telegram send → ApplyInvite + audit row in the same per-
// creator tx (declined-source rows additionally reset reminded_*/decided_at
// so the next decision cycle starts clean); transport failure → the row
// stays put and the failure surfaces in the returned undelivered list.
func (s *CampaignCreatorService) Notify(ctx context.Context, campaignID string, creatorIDs []string) ([]domain.NotifyFailure, error) {
	return s.dispatchBatch(ctx, campaignID, creatorIDs, batchOpNotify)
}

// RemindInvitation bumps reminded_count / reminded_at for every creator in
// the batch without changing the status. Symmetrical to Notify in
// validation, delivery and audit semantics; the only differences are the
// allowed source status (`invited` only) and the audit action
// (`campaign_creator_remind`).
func (s *CampaignCreatorService) RemindInvitation(ctx context.Context, campaignID string, creatorIDs []string) ([]domain.NotifyFailure, error) {
	return s.dispatchBatch(ctx, campaignID, creatorIDs, batchOpRemindInvitation)
}

// dispatchBatch is the shared body of Notify / RemindInvitation: campaign
// gate → batch validation → chat-id resolve → per-creator delivery loop.
// Returns the undelivered list (empty for full success, populated for
// partial-success). Validation failures return CampaignCreatorBatchInvalidError
// with every offending creator, not first-fail.
func (s *CampaignCreatorService) dispatchBatch(ctx context.Context, campaignID string, creatorIDs []string, op batchOp) ([]domain.NotifyFailure, error) {
	spec := batchOpSpecs[op]

	campaign, err := s.getActiveCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	rows, err := s.repoFactory.NewCampaignCreatorRepo(s.pool).
		ListByCampaignAndCreators(ctx, campaignID, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("list campaign creators: %w", err)
	}
	rowByCreator := make(map[string]*repository.CampaignCreatorRow, len(rows))
	for _, row := range rows {
		rowByCreator[row.CreatorID] = row
	}

	var details []domain.BatchValidationDetail
	for _, cid := range creatorIDs {
		row, ok := rowByCreator[cid]
		if !ok {
			details = append(details, domain.BatchValidationDetail{
				CreatorID: cid,
				Reason:    domain.BatchInvalidReasonNotInCampaign,
			})
			continue
		}
		if !spec.allowedStatuses[row.Status] {
			details = append(details, domain.BatchValidationDetail{
				CreatorID:     cid,
				Reason:        domain.BatchInvalidReasonWrongStatus,
				CurrentStatus: row.Status,
			})
		}
	}
	if len(details) > 0 {
		return nil, &domain.CampaignCreatorBatchInvalidError{Details: details}
	}

	chatIDs, err := s.repoFactory.NewCreatorRepo(s.pool).
		GetTelegramUserIDsByIDs(ctx, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("get telegram user ids: %w", err)
	}

	var undelivered []domain.NotifyFailure
	for _, cid := range creatorIDs {
		row := rowByCreator[cid]
		chatID, hasChat := chatIDs[cid]
		if !hasChat {
			// Schema invariant: creators.telegram_user_id is NOT NULL once
			// the application is approved. Reaching this branch means the
			// creator row was hard-deleted between validate-pass and
			// delivery — record as `unknown` so the admin sees the failure.
			undelivered = append(undelivered, domain.NotifyFailure{
				CreatorID: cid,
				Reason:    domain.NotifyFailureReasonUnknown,
			})
			s.logger.Error(ctx, "campaign batch: missing telegram_user_id",
				"campaign_id", campaignID, "creator_id", cid, "op", string(op))
			continue
		}
		if sendErr := s.notifier.SendCampaignInvite(ctx, chatID, spec.text, campaign.TmaURL); sendErr != nil {
			reason := telegram.MapTelegramErrorToReason(sendErr)
			undelivered = append(undelivered, domain.NotifyFailure{
				CreatorID: cid,
				Reason:    reason,
			})
			s.logger.Warn(ctx, "campaign batch: telegram delivery failed",
				"campaign_id", campaignID, "creator_id", cid,
				"op", string(op), "reason", reason)
			continue
		}

		if applyErr := s.applyDelivered(ctx, spec, row); applyErr != nil {
			// Send already happened — the creator received the bot message.
			// Returning an error here would surface 500 to the admin and the
			// already-delivered creator would be retried on the next call,
			// causing duplicate delivery. Instead we report the
			// sent-but-not-persisted state through `undelivered` with reason
			// `unknown` and keep going so the rest of the batch still
			// commits. The error is logged so ops can reconcile manually.
			undelivered = append(undelivered, domain.NotifyFailure{
				CreatorID: cid,
				Reason:    domain.NotifyFailureReasonUnknown,
			})
			s.logger.Error(ctx, "campaign batch: telegram sent but persist failed",
				"campaign_id", campaignID, "creator_id", cid,
				"op", string(op), "error", applyErr.Error())
			continue
		}
	}

	s.logger.Info(ctx, "campaign batch dispatched",
		"campaign_id", campaignID,
		"op", string(op),
		"delivered", len(creatorIDs)-len(undelivered),
		"undelivered", len(undelivered))
	return undelivered, nil
}

// applyDelivered persists one creator's status / counter advance plus the
// audit row in a single per-creator transaction. The op-specific apply
// closure and audit action come from the resolved batchOpSpec, so this
// helper carries no switch on op.
func (s *CampaignCreatorService) applyDelivered(
	ctx context.Context,
	spec batchOpSpec,
	oldRow *repository.CampaignCreatorRow,
) error {
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		ccTx := s.repoFactory.NewCampaignCreatorRepo(tx)
		auditTx := s.repoFactory.NewAuditRepo(tx)

		newRow, err := spec.apply(ccTx, ctx, oldRow.ID)
		if err != nil {
			return fmt.Errorf("apply: %w", err)
		}

		oldCC := campaignCreatorRowToDomain(oldRow)
		newCC := campaignCreatorRowToDomain(newRow)
		return writeAudit(ctx, auditTx,
			spec.auditAction, AuditEntityTypeCampaignCreator, newCC.ID,
			oldCC, newCC)
	})
}

// assertCampaignActive resolves the soft-deleted / missing campaign gate via
// pool (no tx). Mirrors UpdateCampaign's behaviour but returns
// ErrCampaignNotFound for both cases since the chunk-10 endpoints never
// expose soft-deleted rows to the admin UI.
func (s *CampaignCreatorService) assertCampaignActive(ctx context.Context, campaignID string) error {
	_, err := s.getActiveCampaign(ctx, campaignID)
	return err
}

// getActiveCampaign returns the campaign row for chunk-12 flows that need
// tma_url alongside the soft-delete gate. Mirrors assertCampaignActive's
// failure semantics: missing or soft-deleted → ErrCampaignNotFound.
func (s *CampaignCreatorService) getActiveCampaign(ctx context.Context, campaignID string) (*repository.CampaignRow, error) {
	campaign, err := s.repoFactory.NewCampaignRepo(s.pool).GetByID(ctx, campaignID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCampaignNotFound
		}
		return nil, fmt.Errorf("get campaign: %w", err)
	}
	if campaign.IsDeleted {
		return nil, domain.ErrCampaignNotFound
	}
	return campaign, nil
}

func campaignCreatorRowToDomain(row *repository.CampaignCreatorRow) *domain.CampaignCreator {
	return &domain.CampaignCreator{
		ID:            row.ID,
		CampaignID:    row.CampaignID,
		CreatorID:     row.CreatorID,
		Status:        row.Status,
		InvitedAt:     row.InvitedAt,
		InvitedCount:  row.InvitedCount,
		RemindedAt:    row.RemindedAt,
		RemindedCount: row.RemindedCount,
		DecidedAt:     row.DecidedAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
