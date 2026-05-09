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

// ContractExtractor parses placeholder occurrences out of a PDF byte stream.
// Declared in this package (Go convention: accept interfaces, return structs)
// so service tests can swap the real extractor for a mock without taking on
// the ledongthuc/pdf import in the test file.
type ContractExtractor interface {
	ExtractPlaceholders(pdfBytes []byte) ([]contract.Placeholder, error)
}

// CampaignService owns the marketing-campaign lifecycle.
type CampaignService struct {
	pool        dbutil.Pool
	repoFactory CampaignRepoFactory
	extractor   ContractExtractor
	logger      logger.Logger
}

// NewCampaignService creates a new CampaignService.
func NewCampaignService(pool dbutil.Pool, repoFactory CampaignRepoFactory, extractor ContractExtractor, log logger.Logger) *CampaignService {
	return &CampaignService{pool: pool, repoFactory: repoFactory, extractor: extractor, logger: log}
}

// UploadContractTemplateResult is returned to the admin UI after a successful
// PUT /campaigns/{id}/contract-template — it carries the sha256 fingerprint
// and the canonical placeholder set so the UI can render confirmation
// without a follow-up GET.
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

// UploadContractTemplate validates a PDF upload, persists it on the campaign
// row, and writes the audit-log entry — all inside one transaction so a
// rollback wipes both. Validation order mirrors the spec: empty body →
// CONTRACT_REQUIRED, unparseable bytes → CONTRACT_INVALID_PDF, then the
// pure-domain ValidateContractTemplatePDF chain (missing/unknown). The
// audit row carries the sha256 hash + placeholder list + size — never the
// PDF bytes themselves (security.md § PII в логах extends to bulk binary
// payloads).
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

	auditMeta := struct {
		Hash         string   `json:"hash"`
		Placeholders []string `json:"placeholders"`
		SizeBytes    int      `json:"size_bytes"`
	}{
		Hash:         hash,
		Placeholders: domain.KnownContractPlaceholders,
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
		"sha256", hash[:12], // short fingerprint, full hash lives in audit_logs
	)
	return &UploadContractTemplateResult{
		Hash:         hash,
		Placeholders: append([]string(nil), domain.KnownContractPlaceholders...),
	}, nil
}

// GetContractTemplate streams the stored PDF back for download. Soft-deleted
// campaigns surface as ErrCampaignNotFound; live campaigns whose
// contract_template_pdf column is empty (admin never uploaded) surface as
// ErrContractTemplateNotFound — handlers map both to 404 with distinct codes.
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
