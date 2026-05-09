package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
)

const (
	// claimBatchSize ограничивает Phase 1 SELECT — низкие объёмы (~100/EFW)
	// + 4 RPS rate-limit на TrustMe, четыре договора за тик.
	claimBatchSize = 4
	// recoveryBatchSize шире claim'а — Phase 0 побочный, и search-вызовы
	// дешевле send-to-sign.
	recoveryBatchSize = 8
	// almatyTimezone — Asia/Almaty per Decision #13. Загружается лениво,
	// fallback на UTC если tz-database в образе отсутствует.
	almatyTimezone = "Asia/Almaty"
	// audit-action имена в БД формируются как "campaign_creator." + suffix —
	// suffix-ы локальные, чтобы не тащить пакет service в contract. Полные
	// строки совпадают с service.AuditActionCampaignCreatorContract*.
	auditActionInitiated       = "contract_initiated"
	auditActionOrphanRecovered = "contract_orphan_recovered"
	// auditEntityTypeCampaignCreator зашит локально, чтобы пакет contract не
	// зависел от service.AuditEntityType*. Идентичная строка живёт в
	// service/audit_constants.go.
	auditEntityTypeCampaignCreator = "campaign_creator"
	auditActorRoleSystem           = "system"
	// defaultContractName — имя по умолчанию для TrustMe ContractName поля.
	// Конфигурируемо станет, когда появятся per-campaign шаблоны имён.
	defaultContractName = "Договор UGC"
	// trustMeCreatorCompanyName — Requisite.CompanyName для физлица-креатора;
	// per intent Decision #13 «литерал «Креатор» (или ФИО)».
	trustMeCreatorCompanyName = "Креатор"
)

// ContractSenderRepoFactory — подмножество RepoFactory, нужное worker'у.
type ContractSenderRepoFactory interface {
	NewContractsRepo(db dbutil.DB) repository.ContractRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CreatorNotifier отправляет «договор отправлен на подпись» в Telegram.
// Принимает (creatorTGID, contractShortURL). Реализация — fire-and-forget
// goroutine в Telegram.Notifier; ошибки уходят в notifier-логи.
type CreatorNotifier interface {
	NotifyContractSent(ctx context.Context, creatorTelegramUserID int64, contractShortURL string)
}

// CreatorTelegramResolver резолвит telegram_user_id по creator_id. Используется
// для бот-уведомления после COMMIT Phase 3. Через CreatorRepo напрямую — не
// засоряем sender service знанием про domain объекты creators.
type CreatorTelegramResolver interface {
	GetTelegramUserIDsByIDs(ctx context.Context, ids []string) (map[string]int64, error)
}

// ContractSenderService — outbox-worker. RunOnce исполняет один тик: Phase 0
// recovery → Phase 1 claim → Phase 2 render+persist+send → Phase 3 finalize.
// Все сетевые вызовы — вне Tx.
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
}

func NewContractSenderService(
	pool dbutil.Pool,
	repoFactory ContractSenderRepoFactory,
	trustMeClient trustme.Client,
	renderer Renderer,
	creatorResolver CreatorTelegramResolver,
	notifier CreatorNotifier,
	log logger.Logger,
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
	}
}

// RunOnce — один тик worker'а. Не возвращает ошибки наружу: каждая фаза
// логирует свои сбои и идёт дальше; cron-scheduler ничего не должен делать
// с ошибкой, кроме как log + ждать следующего тика.
//
// Defer/recover защищает крон-горутину от panic'ов внутри signintech/gopdf
// или ledongthuc/pdf на битых шаблонах: обвал одного тика не должен ронять
// scheduler.
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

// recoverOrphans (Phase 0): для contracts с trustme_document_id IS NULL
// делает search в TrustMe; если known — finalize, если unknown с PDF —
// re-send без re-render, если unknown без PDF — render+persist+send.
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
		return
	}

	if len(orphan.UnsignedPDFContent) > 0 {
		s.resendOrphan(ctx, orphan.ContractID, orphan.UnsignedPDFContent)
		return
	}

	// Phase 2b упал до persist'а — повторяем render+persist+send как новый
	// claim (но уже claim'нутый ряд). Здесь нужны creator+campaign-данные;
	// SelectAgreedForClaim не поможет, нужен дополнительный запрос —
	// поэтому в проде такой кейс крайне редок (Phase 2b — единичный
	// UPDATE), и принимаем риск что на этом тике recover'нём только
	// finalize/resend ветви, а полный re-render сделает следующий цикл
	// после ручного восстановления `unsigned_pdf_content`. Пока — error
	// log + skip.
	s.logger.Error(ctx, "contract: phase 0 orphan without unsigned pdf — manual intervention needed",
		"contract_id", orphan.ContractID)
}

func (s *ContractSenderService) finalizeKnownOrphan(ctx context.Context, contractID string, search *trustme.SearchContractResult) {
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	requisites, err := contractsRepo.GetOrphanRequisites(ctx, contractID)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 finalize lookup", "contract_id", contractID, "err", err)
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
		return
	}
	s.logger.Info(ctx, "contract: phase 0 recovered known document",
		"contract_id", contractID, "trustme_document_id", search.DocumentID)
	s.notifyCreator(ctx, requisites.CreatorID, search.ShortURL)
}

func (s *ContractSenderService) resendOrphan(ctx context.Context, contractID string, unsignedPDF []byte) {
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	requisites, err := contractsRepo.GetOrphanRequisites(ctx, contractID)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend lookup", "contract_id", contractID, "err", err)
		return
	}

	in := trustme.SendToSignInput{
		PDFBase64:      base64.StdEncoding.EncodeToString(unsignedPDF),
		AdditionalInfo: contractID,
		ContractName:   defaultContractName,
		Requisites: []trustme.Requisite{{
			CompanyName: trustMeCreatorCompanyName,
			FIO:         composeFIO(requisites.CreatorLastName, requisites.CreatorFirstName, requisites.CreatorMiddleName),
			IINBIN:      requisites.CreatorIIN,
			PhoneNumber: domain.NormalizePhoneE164(requisites.CreatorPhone),
		}},
	}
	res, err := s.trustMeClient.SendToSign(ctx, in)
	if err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend send", "contract_id", contractID, "err", err)
		return
	}
	if err := s.finalize(ctx, contractID, requisites.CampaignCreatorID, res, auditActionOrphanRecovered); err != nil {
		s.logger.Error(ctx, "contract: phase 0 resend finalize", "contract_id", contractID, "err", err)
		return
	}
	s.notifyCreator(ctx, requisites.CreatorID, res.ShortURL)
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
	ContractID  string
	CC          *repository.AgreedClaimRow
	UnsignedPDF []byte
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
				ContractID: contract.ID,
				CC:         row,
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
	// Phase 2a — render
	pdf, err := s.renderer.Render(c.CC.ContractTemplatePDF, ContractData{
		CreatorFIO: composeFIO(c.CC.CreatorLastName, c.CC.CreatorFirstName, c.CC.CreatorMiddleName),
		CreatorIIN: c.CC.CreatorIIN,
		IssuedDate: domain.FormatIssuedDate(s.now(), s.loc),
	})
	if err != nil {
		s.logger.Error(ctx, "contract: phase 2a render", "contract_id", c.ContractID, "err", err)
		return
	}

	// Phase 2b — persist
	contractsRepo := s.repoFactory.NewContractsRepo(s.pool)
	if err := contractsRepo.UpdateUnsignedPDF(ctx, c.ContractID, pdf); err != nil {
		s.logger.Error(ctx, "contract: phase 2b persist", "contract_id", c.ContractID, "err", err)
		return
	}

	// Phase 2c — send
	res, err := s.trustMeClient.SendToSign(ctx, trustme.SendToSignInput{
		PDFBase64:      base64.StdEncoding.EncodeToString(pdf),
		AdditionalInfo: c.ContractID,
		ContractName:   defaultContractName,
		Requisites: []trustme.Requisite{{
			CompanyName: trustMeCreatorCompanyName,
			FIO:         composeFIO(c.CC.CreatorLastName, c.CC.CreatorFirstName, c.CC.CreatorMiddleName),
			IINBIN:      c.CC.CreatorIIN,
			PhoneNumber: domain.NormalizePhoneE164(c.CC.CreatorPhone),
		}},
	})
	if err != nil {
		s.logger.Error(ctx, "contract: phase 2c send", "contract_id", c.ContractID, "err", err)
		return
	}

	// Phase 3 — finalize
	if err := s.finalize(ctx, c.ContractID, c.CC.CampaignCreatorID, res, auditActionInitiated); err != nil {
		s.logger.Error(ctx, "contract: phase 3 finalize", "contract_id", c.ContractID, "err", err)
		return
	}

	// Бот-уведомление ПОСЛЕ Tx (стандарт backend-transactions).
	s.notifyCreator(ctx, c.CC.CreatorID, res.ShortURL)
}

func (s *ContractSenderService) finalize(ctx context.Context, contractID, ccID string, res *trustme.SendToSignResult, action string) error {
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		contractsRepo := s.repoFactory.NewContractsRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)
		// trustme_status_code остаётся 0 после успешного SendToSign — TrustMe
		// возвращает первичный документ в статусе «не подписан».
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
	entityID := contractID
	return repo.Create(ctx, repository.AuditLogRow{
		ActorID:    nil,
		ActorRole:  auditActorRoleSystem,
		Action:     "campaign_creator." + action,
		EntityType: auditEntityTypeCampaignCreator,
		EntityID:   &entityID,
		NewValue:   body,
	})
}

func (s *ContractSenderService) notifyCreator(ctx context.Context, creatorID, shortURL string) {
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
	s.notifier.NotifyContractSent(ctx, tgID, shortURL)
}

func composeFIO(last, first string, middle *string) string {
	parts := []string{last, first}
	if middle != nil && strings.TrimSpace(*middle) != "" {
		parts = append(parts, *middle)
	}
	return strings.Join(parts, " ")
}
