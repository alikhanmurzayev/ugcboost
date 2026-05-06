package service

import (
	"context"

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
func (s *CampaignService) CreateCampaign(ctx context.Context, name, tmaURL string) (*domain.Campaign, error) {
	var campaign *domain.Campaign
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		campaignRepo := s.repoFactory.NewCampaignRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := campaignRepo.Create(ctx, name, tmaURL)
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
	s.logger.Info(ctx, "campaign created", "campaign_id", campaign.ID)
	return campaign, nil
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
