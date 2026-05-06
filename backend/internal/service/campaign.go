package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// CampaignRepoFactory creates the repositories needed by CampaignService.
type CampaignRepoFactory interface {
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CampaignService owns the marketing-campaign lifecycle. The current chunk
// only covers admin-initiated creation; downstream chunks (#4–#7) extend it
// with read / update / soft-delete.
type CampaignService struct {
	pool        dbutil.Pool
	repoFactory CampaignRepoFactory
	logger      logger.Logger
}

// NewCampaignService creates a new CampaignService.
func NewCampaignService(pool dbutil.Pool, repoFactory CampaignRepoFactory, log logger.Logger) *CampaignService {
	return &CampaignService{pool: pool, repoFactory: repoFactory, logger: log}
}

// CreateCampaign inserts a new campaign and writes the matching audit row in
// the same transaction. The whole *domain.Campaign is serialized into
// audit_logs.new_value so the payload follows the struct as it grows in
// future chunks without per-callsite changes.
func (s *CampaignService) CreateCampaign(ctx context.Context, in domain.CampaignInput) (*domain.Campaign, error) {
	var campaign *domain.Campaign
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		campaignRepo := s.repoFactory.NewCampaignRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := campaignRepo.Create(ctx, in.Name, in.TmaURL)
		if err != nil {
			return err
		}
		campaign = campaignRowToDomain(row)

		return writeAudit(ctx, auditRepo,
			AuditActionCampaignCreate, AuditEntityTypeCampaign, campaign.ID,
			nil, campaign)
	})
	if err != nil {
		return nil, err
	}
	// Success log lives AFTER WithTx returns so a rolled-back tx never
	// claims success in stdout-логах (backend-transactions.md § Аудит-лог).
	s.logger.Info(ctx, "campaign created", "campaign_id", campaign.ID)
	return campaign, nil
}

// UpdateCampaign full-replaces name/tma_url and writes a campaign_update audit
// row in the same tx. Pre-fetch via GetByID feeds audit_logs.old_value;
// soft-deleted rows are refused with ErrCampaignNotFound (gate is here, not repo).
func (s *CampaignService) UpdateCampaign(ctx context.Context, id string, in domain.CampaignInput) error {
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		campaignRepo := s.repoFactory.NewCampaignRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		oldRow, err := campaignRepo.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignNotFound
			}
			return fmt.Errorf("get campaign: %w", err)
		}
		if oldRow.IsDeleted {
			return domain.ErrCampaignNotFound
		}
		oldCampaign := campaignRowToDomain(oldRow)

		newRow, err := campaignRepo.Update(ctx, id, in.Name, in.TmaURL)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignNotFound
			}
			if errors.Is(err, domain.ErrCampaignNameTaken) {
				return err
			}
			return fmt.Errorf("update campaign: %w", err)
		}
		newCampaign := campaignRowToDomain(newRow)

		return writeAudit(ctx, auditRepo,
			AuditActionCampaignUpdate, AuditEntityTypeCampaign, newCampaign.ID,
			oldCampaign, newCampaign)
	})
	if err != nil {
		return err
	}
	s.logger.Info(ctx, "campaign updated", "campaign_id", id)
	return nil
}

// GetByID fetches a campaign by id. The read runs against the pool directly —
// no transaction, no audit, no success log (read paths stay quiet per
// docs/standards/security.md). sql.ErrNoRows from the repo is translated into
// domain.ErrCampaignNotFound at the boundary so the handler maps it to 404
// CAMPAIGN_NOT_FOUND rather than the generic NOT_FOUND fallback. The repo
// returns soft-deleted rows untouched — admins see and audit them.
func (s *CampaignService) GetByID(ctx context.Context, id string) (*domain.Campaign, error) {
	row, err := s.repoFactory.NewCampaignRepo(s.pool).GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCampaignNotFound
		}
		return nil, fmt.Errorf("get campaign: %w", err)
	}
	return campaignRowToDomain(row), nil
}

func campaignRowToDomain(row *repository.CampaignRow) *domain.Campaign {
	return &domain.Campaign{
		ID:        row.ID,
		Name:      row.Name,
		TmaURL:    row.TmaURL,
		IsDeleted: row.IsDeleted,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
