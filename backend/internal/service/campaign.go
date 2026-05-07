package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// AssertActiveCampaigns checks that every id refers to an existing,
// non-soft-deleted campaign. Empty input is a noop — handler short-circuits
// the optional `campaignIds` payload before calling. The check runs against
// the pool (no tx) before ApproveApplication opens its own transaction, so a
// missing or soft-deleted campaign aborts the approve before any creator row
// or audit row is written. Race between this check and the per-campaign add
// loop is caught by ErrCampaignNotFound from CampaignCreatorService.Add
// (defense in depth).
func (s *CampaignService) AssertActiveCampaigns(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.repoFactory.NewCampaignRepo(s.pool).ListByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("list campaigns by ids: %w", err)
	}
	if len(rows) != len(ids) {
		return domain.ErrCampaignNotAvailableForAdd
	}
	for _, row := range rows {
		if row.IsDeleted {
			return domain.ErrCampaignNotAvailableForAdd
		}
	}
	return nil
}

// List returns a page of campaigns matching the validated filter set. The
// handler enforces sort/order whitelists and page/perPage bounds; this
// method trusts those invariants, trims the optional search and runs the
// repo's page+count query against the pool. No transaction (cross-row
// consistency on the order of milliseconds is not required for an admin
// list read), no audit, no success log.
func (s *CampaignService) List(ctx context.Context, in domain.CampaignListInput) (*domain.CampaignListPage, error) {
	params := campaignListInputToRepo(in)

	rows, total, err := s.repoFactory.NewCampaignRepo(s.pool).List(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	if total == 0 || len(rows) == 0 {
		return &domain.CampaignListPage{
			Items:   nil,
			Total:   total,
			Page:    in.Page,
			PerPage: in.PerPage,
		}, nil
	}

	items := make([]*domain.Campaign, len(rows))
	for i, row := range rows {
		items[i] = campaignRowToDomain(row)
	}
	return &domain.CampaignListPage{
		Items:   items,
		Total:   total,
		Page:    in.Page,
		PerPage: in.PerPage,
	}, nil
}

func campaignListInputToRepo(in domain.CampaignListInput) repository.CampaignListParams {
	return repository.CampaignListParams{
		Search:    strings.ToLower(strings.TrimSpace(in.Search)),
		IsDeleted: in.IsDeleted,
		Sort:      in.Sort,
		Order:     in.Order,
		Page:      in.Page,
		PerPage:   in.PerPage,
	}
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
