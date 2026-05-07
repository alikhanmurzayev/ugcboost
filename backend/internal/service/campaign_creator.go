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
)

// CampaignCreatorRepoFactory creates the repositories CampaignCreatorService
// needs. The campaign repo is here only for the soft-delete pre-fetch — the
// chunk-10 endpoints share the "soft-deleted = 404" gate that the per-id
// read deliberately does not enforce.
type CampaignCreatorRepoFactory interface {
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CampaignCreatorService owns the admin-only attachment lifecycle for chunk
// 10: batch add → planned, single remove (forbidden once agreed), and the
// no-pagination list. State transitions outside `→ planned` land in chunks
// 12 (notify / remind) and 14 (TMA agree / decline).
type CampaignCreatorService struct {
	pool        dbutil.Pool
	repoFactory CampaignCreatorRepoFactory
	logger      logger.Logger
}

// NewCampaignCreatorService creates a new CampaignCreatorService.
func NewCampaignCreatorService(pool dbutil.Pool, repoFactory CampaignCreatorRepoFactory, log logger.Logger) *CampaignCreatorService {
	return &CampaignCreatorService{pool: pool, repoFactory: repoFactory, logger: log}
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

// assertCampaignActive resolves the soft-deleted / missing campaign gate via
// pool (no tx). Mirrors UpdateCampaign's behaviour but returns
// ErrCampaignNotFound for both cases since the chunk-10 endpoints never
// expose soft-deleted rows to the admin UI.
func (s *CampaignCreatorService) assertCampaignActive(ctx context.Context, campaignID string) error {
	campaign, err := s.repoFactory.NewCampaignRepo(s.pool).GetByID(ctx, campaignID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrCampaignNotFound
		}
		return fmt.Errorf("get campaign: %w", err)
	}
	if campaign.IsDeleted {
		return domain.ErrCampaignNotFound
	}
	return nil
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
