package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/contract"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// CampaignRepoFactory creates the repositories needed by CampaignService.
// The campaign_creators repo backs the PATCH lock — UpdateCampaign refuses
// tma_url changes once any creator in the campaign has been invited.
type CampaignRepoFactory interface {
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

type ContractExtractor interface {
	ExtractPlaceholders(pdfBytes []byte) ([]contract.Placeholder, error)
}

type CampaignService struct {
	pool        dbutil.Pool
	repoFactory CampaignRepoFactory
	extractor   ContractExtractor
	logger      logger.Logger
}

func NewCampaignService(pool dbutil.Pool, repoFactory CampaignRepoFactory, extractor ContractExtractor, log logger.Logger) *CampaignService {
	return &CampaignService{pool: pool, repoFactory: repoFactory, extractor: extractor, logger: log}
}

type UploadContractTemplateResult struct {
	Hash         string
	Placeholders []string
}

// CreateCampaign inserts a new campaign and writes the matching audit row in
// the same transaction. The whole *domain.Campaign is serialized into
// audit_logs.new_value so the payload follows the struct without
// per-callsite changes. Empty TmaURL stores secret_token=NULL — the row
// stays admin-reachable but invisible to the TMA flow until the URL is
// filled in.
func (s *CampaignService) CreateCampaign(ctx context.Context, in domain.CampaignInput) (*domain.Campaign, error) {
	secretToken := domain.ExtractSecretToken(in.TmaURL)
	var campaign *domain.Campaign
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		campaignRepo := s.repoFactory.NewCampaignRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := campaignRepo.Create(ctx, in.Name, in.TmaURL, secretToken)
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
//
// tma_url lock: when the request flips tma_url to a new value, we refuse
// the change if any creator in this campaign has been invited at least
// once. The previous URL is already embedded in inline `web_app` buttons
// of bot messages delivered to creators; flipping it would silently break
// those links. The check uses ExistsInvitedInCampaign on the same tx as
// the UPDATE on `campaigns`. Residual race under READ COMMITTED: a
// concurrent Notify can INSERT/UPDATE `campaign_creators` between our
// EXISTS read and the UPDATE on `campaigns`. Closing it would require a
// `SELECT FOR UPDATE` on the campaigns row in both Notify and PATCH.
// No-op (tma_url unchanged) bypasses the lock entirely.
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

		if in.TmaURL != oldRow.TmaURL {
			locked, err := s.repoFactory.NewCampaignCreatorRepo(tx).ExistsInvitedInCampaign(ctx, id)
			if err != nil {
				return fmt.Errorf("check tma_url lock: %w", err)
			}
			if locked {
				return domain.ErrCampaignTmaURLLocked
			}
		}

		newSecretToken := domain.ExtractSecretToken(in.TmaURL)
		newRow, err := campaignRepo.Update(ctx, id, in.Name, in.TmaURL, newSecretToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignNotFound
			}
			if errors.Is(err, domain.ErrCampaignNameTaken) || errors.Is(err, domain.ErrTmaURLConflict) {
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
//
// Membership uses an explicit set so duplicate or phantom rows (in theory
// impossible behind the campaigns PK, but defensive against a future relaxed
// query) cannot inflate len(rows) past len(ids) and silently pass the gate.
func (s *CampaignService) AssertActiveCampaigns(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.repoFactory.NewCampaignRepo(s.pool).ListByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("list campaigns by ids: %w", err)
	}
	active := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.IsDeleted {
			return domain.ErrCampaignNotAvailableForAdd
		}
		active[row.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := active[id]; !ok {
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

func (s *CampaignService) UploadContractTemplate(ctx context.Context, id string, pdf []byte) (*UploadContractTemplateResult, error) {
	if len(pdf) == 0 {
		return nil, domain.NewContractRequiredError()
	}

	placeholders, err := s.extractor.ExtractPlaceholders(pdf)
	if err != nil {
		return nil, domain.NewContractInvalidPDFError()
	}

	names := make([]string, len(placeholders))
	for i, p := range placeholders {
		names[i] = p.Name
	}
	if err := domain.ValidateContractTemplatePDF(len(pdf), names); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(pdf)
	hash := hex.EncodeToString(sum[:])

	auditPlaceholders := dedupNames(names)

	auditMeta := struct {
		Hash         string   `json:"hash"`
		Placeholders []string `json:"placeholders"`
		SizeBytes    int      `json:"size_bytes"`
	}{
		Hash:         hash,
		Placeholders: auditPlaceholders,
		SizeBytes:    len(pdf),
	}

	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		campaignRepo := s.repoFactory.NewCampaignRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		if err := campaignRepo.UpdateContractTemplate(ctx, id, pdf); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrCampaignNotFound
			}
			return fmt.Errorf("update contract template: %w", err)
		}
		return writeAudit(ctx, auditRepo,
			AuditActionCampaignContractTemplateUploaded, AuditEntityTypeCampaign, id,
			nil, auditMeta)
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(ctx, "contract template uploaded",
		"campaign_id", id,
		"size_bytes", len(pdf),
		"sha256", hash[:12],
	)
	return &UploadContractTemplateResult{
		Hash:         hash,
		Placeholders: append([]string(nil), domain.KnownContractPlaceholders...),
	}, nil
}

func (s *CampaignService) GetContractTemplate(ctx context.Context, id string) ([]byte, error) {
	pdf, err := s.repoFactory.NewCampaignRepo(s.pool).GetContractTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCampaignNotFound
		}
		return nil, fmt.Errorf("get contract template: %w", err)
	}
	if len(pdf) == 0 {
		return nil, domain.ErrContractTemplateNotFound
	}
	return pdf, nil
}

func dedupNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
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
		ID:                  row.ID,
		Name:                row.Name,
		TmaURL:              row.TmaURL,
		IsDeleted:           row.IsDeleted,
		HasContractTemplate: row.HasContractTemplate,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}
