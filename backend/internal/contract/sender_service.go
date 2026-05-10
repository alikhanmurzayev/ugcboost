package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
)

const (
	// claimBatchSize синхронизирован с TrustMe rate-limit 4 RPS — четыре
	// договора за тик.
	claimBatchSize    = 4
	recoveryBatchSize = 8
	// almatyTimezone — Asia/Almaty per Decision #13.
	almatyTimezone = "Asia/Almaty"
	// audit-action имена локально продублированы, чтобы contract не тянул
	// service. Должны совпадать со service.AuditActionCampaignCreatorContract*.
	auditActionInitiated           = "contract_initiated"
	auditActionOrphanRecovered     = "contract_orphan_recovered"
	auditEntityTypeCampaignCreator = "campaign_creator"
	auditActorRoleSystem           = "system"
	defaultContractName            = "Договор UGC"
	contractNumberPrefix           = "UGC-"
)

// ContractSenderRepoFactory — подмножество RepoFactory, нужное worker'у.
type ContractSenderRepoFactory interface {
	NewContractsRepo(db dbutil.DB) repository.ContractRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CreatorNotifier — Telegram-уведомление «договор отправлен». Реализация
// fire-and-forget; shortURL не передаём, TrustMe сам присылает SMS.
type CreatorNotifier interface {
	NotifyContractSent(ctx context.Context, creatorTelegramUserID int64)
}

// CreatorTelegramResolver резолвит telegram_user_id по creator_id для
// post-commit бот-уведомления.
type CreatorTelegramResolver interface {
	GetTelegramUserIDsByIDs(ctx context.Context, ids []string) (map[string]int64, error)
}

// ContractSenderService — outbox-worker. RunOnce: Phase 0 recovery → Phase 1
// claim → Phase 2 render+persist+send → Phase 3 finalize. Сетевые вызовы вне Tx.
type ContractSenderService struct {
	pool            dbutil.Pool
	repoFactory     ContractSenderRepoFactory
	trustMeClient   trustme.Client
	renderer        Renderer
	creatorResolver CreatorTelegramResolver
	notifier        CreatorNotifier
	logger          logger.Logger
	now             func() time.Time
	loc             *time.Location
	retryBackoff    time.Duration
}

func NewContractSenderService(
	pool dbutil.Pool,
	repoFactory ContractSenderRepoFactory,
	trustMeClient trustme.Client,
	renderer Renderer,
	creatorResolver CreatorTelegramResolver,
	notifier CreatorNotifier,
	log logger.Logger,
	retryBackoff time.Duration,
) *ContractSenderService {
	loc, err := time.LoadLocation(almatyTimezone)
	if err != nil {
		loc = time.UTC
	}
	return &ContractSenderService{
		pool:            pool,
		repoFactory:     repoFactory,
		trustMeClient:   trustMeClient,
		renderer:        renderer,
		creatorResolver: creatorResolver,
		notifier:        notifier,
		logger:          log,
		now:             func() time.Time { return time.Now().UTC() },
		loc:             loc,
		retryBackoff:    retryBackoff,
	}
}

// RunOnce — один тик. Ошибки наружу не пробрасываются (cron-scheduler сам
// решает только когда триггерить следующий тик). defer/recover страхует от
// panic'ов внутри gopdf/ledongthuc на битых PDF — sender'у нельзя ронять весь
// scheduler. На уровне самого scheduler'а второй слой защиты — cron.Recover.
func (s *ContractSenderService) RunOnce(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error(ctx, "contract: RunOnce recovered from panic", "panic", r)
		}
	}()
	s.recoverOrphans(ctx)
	s.processAgreed(ctx)
}

// recoverOrphans (Phase 0): для contracts с trustme_document_id IS NULL —
// search в TrustMe; known → finalize, unknown с PDF → re-send без re-render,
// unknown без PDF → log+backoff (manual intervention).
func (s *ContractSenderService) recoverOrphans(ctx context.Context) {
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	orphans, err := contractsRepo.SelectOrphansForRecovery(ctx, recoveryBatchSize)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 select orphans", "err", err)
		return
	}
	for _, o := range orphans {
		s.recoverOne(ctx, o)
	}
}

func (s *ContractSenderService) recoverOne(ctx context.Context, orphan *repository.OrphanRow) {
	search, err := s.trustMeClient.SearchContractByAdditionalInfo(ctx, orphan.ContractID)
	switch {
	case err == nil && search != nil:
		s.finalizeKnownOrphan(ctx, orphan.ContractID, search)
		return
	case errors.Is(err, trustme.ErrTrustMeNotFound):
		// fall through — TrustMe не знает, перепосылаем
	default:
		s.logger.Error(ctx, "contract: phase 0 search", "contract_id", orphan.ContractID, "err", err)
		s.recordFailedAttempt(ctx, orphan.ContractID, err)
		return
	}

	if len(orphan.UnsignedPDFContent) > 0 {
		s.resendOrphan(ctx, orphan.ContractID, orphan.UnsignedPDFContent)
		return
	}

	// Phase 2b упал до persist'а — unsigned_pdf отсутствует, нужно ручное
	// восстановление. Backoff на next_retry_at не даёт log-спама на каждом тике.
	s.logger.Error(ctx, "contract: phase 0 orphan without unsigned pdf — manual intervention needed",
		"contract_id", orphan.ContractID)
	s.recordFailedAttempt(ctx, orphan.ContractID, errors.New("manual intervention: orphan without unsigned pdf"))
}

func (s *ContractSenderService) finalizeKnownOrphan(ctx context.Context, contractID string, search *trustme.SearchContractResult) {
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	requisites, err := contractsRepo.GetOrphanRequisites(ctx, contractID)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 finalize lookup", "contract_id", contractID, "err", err)
		s.recordFailedAttempt(ctx, contractID, err)
		return
	}

	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		txContracts := s.repoFactory.NewContractsRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)
		if err := txContracts.UpdateAfterSend(ctx, contractID, search.DocumentID, search.ShortURL, search.ContractStatus); err != nil {
			return fmt.Errorf("update after send: %w", err)
		}
		if err := s.recordAudit(ctx, auditRepo, contractID, requisites.CampaignCreatorID, auditActionOrphanRecovered, search); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 finalize-known", "contract_id", contractID, "err", err)
		s.recordFailedAttempt(ctx, contractID, err)
		return
	}
	s.logger.Info(ctx, "contract: phase 0 recovered known document",
		"contract_id", contractID, "trustme_document_id", search.DocumentID)
	s.notifyCreator(ctx, requisites.CreatorID)
}

func (s *ContractSenderService) resendOrphan(ctx context.Context, contractID string, unsignedPDF []byte) {
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	requisites, err := contractsRepo.GetOrphanRequisites(ctx, contractID)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend lookup", "contract_id", contractID, "err", err)
		s.recordFailedAttempt(ctx, contractID, err)
		return
	}

	fio := composeFIO(requisites.CreatorLastName, requisites.CreatorFirstName, requisites.CreatorMiddleName)
	in := trustme.SendToSignInput{
		PDFBase64:      base64.StdEncoding.EncodeToString(unsignedPDF),
		AdditionalInfo: contractID,
		ContractName:   defaultContractName,
		NumberDial:     formatContractNumber(requisites.SerialNumber),
		Requisites: []trustme.Requisite{{
			CompanyName: fio,
			FIO:         fio,
			IINBIN:      requisites.CreatorIIN,
			PhoneNumber: domain.NormalizePhoneE164(requisites.CreatorPhone),
		}},
	}
	res, err := s.trustMeClient.SendToSign(ctx, in)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend send", "contract_id", contractID, "err", err)
		s.recordFailedAttempt(ctx, contractID, err)
		return
	}
	if err := s.finalize(ctx, contractID, requisites.CampaignCreatorID, res, auditActionOrphanRecovered); err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend finalize", "contract_id", contractID, "err", err)
		s.recordFailedAttempt(ctx, contractID, err)
		return
	}
	s.notifyCreator(ctx, requisites.CreatorID)
}

// processAgreed (Phase 1+2+3) — основной flow.
func (s *ContractSenderService) processAgreed(ctx context.Context) {
	claims, err := s.claimAgreedBatch(ctx)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 1 claim", "err", err)
		return
	}
	for _, claim := range claims {
		s.processClaim(ctx, claim)
	}
}

// claim — состояние от Phase 1, переходящее в Phase 2/3.
type claim struct {
	ContractID   string
	SerialNumber int64
	CC           *repository.AgreedClaimRow
	UnsignedPDF  []byte
}

func (s *ContractSenderService) claimAgreedBatch(ctx context.Context) ([]claim, error) {
	var out []claim
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		contractsRepo := s.repoFactory.NewContractsRepo(tx)
		ccRepo := s.repoFactory.NewCampaignCreatorRepo(tx)

		rows, err := contractsRepo.SelectAgreedForClaim(ctx, claimBatchSize)
		if err != nil {
			return fmt.Errorf("select agreed: %w", err)
		}
		for _, row := range rows {
			contract, err := contractsRepo.Insert(ctx, repository.ContractRow{
				SubjectKind:       repository.ContractSubjectKindCampaignCreator,
				TrustMeStatusCode: 0,
			})
			if err != nil {
				return fmt.Errorf("insert contract: %w", err)
			}
			if err := ccRepo.UpdateContractIDAndStatus(ctx, row.CampaignCreatorID, contract.ID, domain.CampaignCreatorStatusSigning); err != nil {
				return fmt.Errorf("update cc: %w", err)
			}
			out = append(out, claim{
				ContractID:   contract.ID,
				SerialNumber: contract.SerialNumber,
				CC:           row,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ContractSenderService) processClaim(ctx context.Context, c claim) {
	fio := composeFIO(c.CC.CreatorLastName, c.CC.CreatorFirstName, c.CC.CreatorMiddleName)

	// Phase 2a — render
	pdf, err := s.renderer.Render(c.CC.ContractTemplatePDF, ContractData{
		CreatorFIO: fio,
		CreatorIIN: c.CC.CreatorIIN,
		IssuedDate: domain.FormatIssuedDate(s.now(), s.loc),
	})
	if err != nil {
		s.logger.Error(ctx, "contract: phase 2a render", "contract_id", c.ContractID, "err", err)
		s.recordFailedAttempt(ctx, c.ContractID, err)
		return
	}

	// Phase 2b — persist
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	if err := contractsRepo.UpdateUnsignedPDF(ctx, c.ContractID, pdf); err != nil {
		s.logger.Error(ctx, "contract: phase 2b persist", "contract_id", c.ContractID, "err", err)
		s.recordFailedAttempt(ctx, c.ContractID, err)
		return
	}

	// Phase 2c — send
	res, err := s.trustMeClient.SendToSign(ctx, trustme.SendToSignInput{
		PDFBase64:      base64.StdEncoding.EncodeToString(pdf),
		AdditionalInfo: c.ContractID,
		ContractName:   defaultContractName,
		NumberDial:     formatContractNumber(c.SerialNumber),
		Requisites: []trustme.Requisite{{
			CompanyName: fio,
			FIO:         fio,
			IINBIN:      c.CC.CreatorIIN,
			PhoneNumber: domain.NormalizePhoneE164(c.CC.CreatorPhone),
		}},
	})
	if err != nil {
		s.logger.Error(ctx, "contract: phase 2c send", "contract_id", c.ContractID, "err", err)
		s.recordFailedAttempt(ctx, c.ContractID, err)
		return
	}

	// Phase 3 — finalize. При сбое document_id у TrustMe уже выдан, но
	// локально не записан — Phase 0 next tick подберёт через search.
	if err := s.finalize(ctx, c.ContractID, c.CC.CampaignCreatorID, res, auditActionInitiated); err != nil {
		s.logger.Error(ctx, "contract: phase 3 finalize", "contract_id", c.ContractID, "err", err)
		s.recordFailedAttempt(ctx, c.ContractID, err)
		return
	}

	// Бот-уведомление ПОСЛЕ Tx (стандарт backend-transactions).
	s.notifyCreator(ctx, c.CC.CreatorID)
}

func (s *ContractSenderService) finalize(ctx context.Context, contractID, ccID string, res *trustme.SendToSignResult, action string) error {
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		contractsRepo := s.repoFactory.NewContractsRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)
		// trustme_status_code=0 — первичный «не подписан».
		if err := contractsRepo.UpdateAfterSend(ctx, contractID, res.DocumentID, res.ShortURL, 0); err != nil {
			return fmt.Errorf("update after send: %w", err)
		}
		if err := s.recordAudit(ctx, auditRepo, contractID, ccID, action,
			&trustme.SearchContractResult{
				DocumentID: res.DocumentID,
				ShortURL:   res.ShortURL,
			}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
}

// recordAudit — EntityType="campaign_creator" + EntityID=ccID (а НЕ
// contractID): action в жизненном цикле заявки креатора, read-side queries
// фильтруют по cc.ID. contract_id/document_id уходят в JSON payload.
func (s *ContractSenderService) recordAudit(ctx context.Context, repo repository.AuditRepo, contractID, ccID, action string, search *trustme.SearchContractResult) error {
	payload := map[string]string{
		"contract_id": contractID,
	}
	if ccID != "" {
		payload["campaign_creator_id"] = ccID
	}
	if search != nil && search.DocumentID != "" {
		payload["trustme_document_id"] = search.DocumentID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	entityID := ccID
	return repo.Create(ctx, repository.AuditLogRow{
		ActorID:    nil,
		ActorRole:  auditActorRoleSystem,
		Action:     "campaign_creator." + action,
		EntityType: auditEntityTypeCampaignCreator,
		EntityID:   &entityID,
		NewValue:   body,
	})
}

// recordFailedAttempt — last_error_* + next_retry_at += retryBackoff. Code
// извлекается только если err — *trustme.Error; сетевые сбои оставляют ""
// в last_error_code.
func (s *ContractSenderService) recordFailedAttempt(ctx context.Context, contractID string, sendErr error) {
	var code string
	var trustMeErr *trustme.Error
	if errors.As(sendErr, &trustMeErr) {
		code = trustMeErr.Code
	}
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	nextRetryAt := s.now().Add(s.retryBackoff)
	if err := contractsRepo.RecordFailedAttempt(ctx, contractID, code, sendErr.Error(), nextRetryAt); err != nil {
		s.logger.Error(ctx, "contract: record failed attempt", "contract_id", contractID, "err", err)
	}
}

func (s *ContractSenderService) notifyCreator(ctx context.Context, creatorID string) {
	if s.notifier == nil || s.creatorResolver == nil {
		return
	}
	tgIDs, err := s.creatorResolver.GetTelegramUserIDsByIDs(ctx, []string{creatorID})
	if err != nil {
		s.logger.Error(ctx, "contract: notify lookup", "creator_id", creatorID, "err", err)
		return
	}
	tgID, ok := tgIDs[creatorID]
	if !ok || tgID == 0 {
		return
	}
	s.notifier.NotifyContractSent(ctx, tgID)
}

func composeFIO(last, first string, middle *string) string {
	parts := []string{capitalizeFirstRune(last), capitalizeFirstRune(first)}
	if middle != nil {
		if m := strings.TrimSpace(*middle); m != "" {
			parts = append(parts, capitalizeFirstRune(m))
		}
	}
	return strings.Join(parts, " ")
}

// capitalizeFirstRune приводит первую руну к верхнему регистру (русский,
// казахский, латиница работают через unicode.ToUpper). Остальные руны
// не трогаем — оставляем как ввёл пользователь (двойные «иванов-петров»
// или uppercase в середине не нормализуем).
func capitalizeFirstRune(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r, sz := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	upper := unicode.ToUpper(r)
	if upper == r {
		return s
	}
	return string(upper) + s[sz:]
}

// formatContractNumber переводит contracts.serial_number в TrustMe NumberDial
// формата UGC-{n}.
func formatContractNumber(serial int64) string {
	return contractNumberPrefix + strconv.FormatInt(serial, 10)
}
