package contract

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// audit-action суффиксы webhook'а — продублированы локально, чтобы contract
// не тянул service. Должны совпадать со service.AuditActionCampaignCreator
// Contract* (префикс `campaign_creator.` добавляется в recordAudit).
const (
	auditActionWebhookSignedSuffix           = "contract_signed"
	auditActionWebhookSigningDeclinedSuffix  = "contract_signing_declined"
	auditActionWebhookUnexpectedStatusSuffix = "contract_unexpected_status"
)

// NotifyKind определяет, какое сообщение отправлять креатору после COMMIT.
// soft-deleted кампания → NotifyKindNone (state-transition + audit пишутся,
// notify пропускается).
type NotifyKind int

const (
	NotifyKindNone NotifyKind = iota
	NotifyKindSigned
	NotifyKindDeclined
)

// WebhookRepoFactory — подмножество repository.RepoFactory, нужное webhook-
// сервису. Сужаем интерфейс по convention accept interfaces, return structs.
type WebhookRepoFactory interface {
	NewContractsRepo(db dbutil.DB) repository.ContractRepo
	NewCampaignCreatorRepo(db dbutil.DB) repository.CampaignCreatorRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// WebhookNotifier — узкий интерфейс для post-Tx fire-and-forget уведомлений
// креатору. Реализация — *telegram.Notifier. signed несёт tmaURL для inline
// WebApp-кнопки с ТЗ; declined — без кнопки.
type WebhookNotifier interface {
	NotifyCampaignContractSigned(ctx context.Context, chatID int64, tmaURL string)
	NotifyCampaignContractDeclined(ctx context.Context, chatID int64)
}

// WebhookService — приёмник TrustMe webhook'а. HandleEvent идемпотентно
// (двойной guard в SQL: idempotency `!= newStatus` + terminal-guard
// `NOT IN (3,4,9)`) обновляет contracts + cc.status (для terminal 3/4/9) +
// audit, всё в одной Tx. Бот-уведомление шлётся ПОСЛЕ COMMIT — стандарт
// backend-transactions «Логи успеха пишутся ПОСЛЕ WithTx».
type WebhookService struct {
	pool        dbutil.Pool
	repoFactory WebhookRepoFactory
	notifier    WebhookNotifier
	logger      logger.Logger
	now         func() time.Time
}

// NewWebhookService собирает иммутабельный сервис. now-инжектится для
// тестов; production wires time.Now.
func NewWebhookService(
	pool dbutil.Pool,
	repoFactory WebhookRepoFactory,
	notifier WebhookNotifier,
	log logger.Logger,
	now func() time.Time,
) *WebhookService {
	return &WebhookService{
		pool:        pool,
		repoFactory: repoFactory,
		notifier:    notifier,
		logger:      log,
		now:         now,
	}
}

// HandleEvent — основная точка входа. Все DB-операции внутри одной Tx;
// после COMMIT — fire-and-forget notify, если NotifyKind != None.
//
// Sentinel-ошибки:
//   - sql.ErrNoRows на LockByTrustMeDocumentID → ErrContractWebhookUnknownDocument (404).
//   - subject_kind != 'campaign_creator' → ErrContractWebhookUnknownSubject (422).
//
// Idempotent повтор / terminal-guard: UPDATE 0 affected → NotifyKind=None,
// no audit, no cc.status mutation. Stale-after-terminal вешает info-лог
// `stale_webhook_after_terminal`.
func (s *WebhookService) HandleEvent(ctx context.Context, ev domain.TrustMeWebhookEvent) error {
	var (
		notifyKind     NotifyKind
		telegramUserID int64
		tmaURL         string
	)
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		contractsRepo := s.repoFactory.NewContractsRepo(tx)
		contractRow, err := contractsRepo.LockByTrustMeDocumentID(ctx, ev.ContractID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrContractWebhookUnknownDocument
			}
			return fmt.Errorf("lock contract: %w", err)
		}
		switch contractRow.SubjectKind {
		case repository.ContractSubjectKindCampaignCreator:
			kind, tgID, url, err := s.applyCampaignCreatorTransition(ctx, tx, contractRow, ev)
			if err != nil {
				return err
			}
			notifyKind = kind
			telegramUserID = tgID
			tmaURL = url
			return nil
		default:
			s.logger.Warn(ctx, "trustme webhook: unknown subject_kind",
				"contract_id", contractRow.ID,
				"subject_kind", contractRow.SubjectKind,
			)
			return domain.ErrContractWebhookUnknownSubject
		}
	})
	if err != nil {
		return err
	}
	switch notifyKind {
	case NotifyKindSigned:
		s.notifier.NotifyCampaignContractSigned(ctx, telegramUserID, tmaURL)
	case NotifyKindDeclined:
		s.notifier.NotifyCampaignContractDeclined(ctx, telegramUserID)
	}
	return nil
}

// applyCampaignCreatorTransition обрабатывает 'campaign_creator' subject_kind:
//
//   - UPDATE contracts с двойным guard'ом (idempotency + terminal-guard).
//   - 0 affected → NotifyKindNone, info-лог `stale_webhook_after_terminal`
//     если row уже в terminal.
//   - 2-step lookup: JOIN cc + campaigns + creators проектирует cc.id +
//     c.is_deleted + c.tma_url + cr.telegram_user_id.
//   - terminal status (3/4/9) → cc.status flips, notifyKind set. Статусы
//     4 (revoked) и 9 (signing_declined) склеены: одинаковая cc.status
//     (signing_declined), одинаковый audit-action и одинаковый текст бота.
//   - intermediate status (0/2) → info-log; неожиданный (1/5-8) → warn-log.
//   - audit row внутри Tx.
//   - soft-deleted кампания → notifyKind=None после state+audit (factual record).
//
// Возвращает (notifyKind, creatorTelegramUserID, campaignTmaURL, err).
// tmaURL пробрасывается в NotifyCampaignContractSigned для inline WebApp-
// кнопки; для declined и intermediate ветвей значение игнорируется.
func (s *WebhookService) applyCampaignCreatorTransition(
	ctx context.Context,
	tx dbutil.DB,
	contractRow *repository.ContractRow,
	ev domain.TrustMeWebhookEvent,
) (NotifyKind, int64, string, error) {
	contractsRepo := s.repoFactory.NewContractsRepo(tx)
	n, err := contractsRepo.UpdateAfterWebhook(ctx, contractRow.ID, ev.Status)
	if err != nil {
		return NotifyKindNone, 0, "", fmt.Errorf("update contract: %w", err)
	}
	if n == 0 {
		if contractRow.TrustMeStatusCode == repository.TrustMeStatusSigned ||
			contractRow.TrustMeStatusCode == repository.TrustMeStatusRevoked ||
			contractRow.TrustMeStatusCode == repository.TrustMeStatusSigningDeclined {
			s.logger.Info(ctx, "trustme webhook: stale_webhook_after_terminal",
				"contract_id", contractRow.ID,
				"trustme_status_code_old", contractRow.TrustMeStatusCode,
				"trustme_status_code_new", ev.Status,
			)
		}
		return NotifyKindNone, 0, "", nil
	}

	ccRepo := s.repoFactory.NewCampaignCreatorRepo(tx)
	view, err := ccRepo.GetWithCampaignAndCreatorByContractID(ctx, contractRow.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.logger.Warn(ctx, "trustme webhook: cc-row missing for contract",
				"contract_id", contractRow.ID,
			)
			return NotifyKindNone, 0, "", domain.ErrContractWebhookUnknownSubject
		}
		return NotifyKindNone, 0, "", fmt.Errorf("lookup cc: %w", err)
	}

	var (
		notifyKind   NotifyKind
		actionSuffix string
	)
	switch ev.Status {
	case repository.TrustMeStatusSigned:
		if err := ccRepo.UpdateStatus(ctx, view.CampaignCreatorID, domain.CampaignCreatorStatusSigned); err != nil {
			return NotifyKindNone, 0, "", fmt.Errorf("update cc status: %w", err)
		}
		actionSuffix = auditActionWebhookSignedSuffix
		notifyKind = NotifyKindSigned
	case repository.TrustMeStatusRevoked, repository.TrustMeStatusSigningDeclined:
		if err := ccRepo.UpdateStatus(ctx, view.CampaignCreatorID, domain.CampaignCreatorStatusSigningDeclined); err != nil {
			return NotifyKindNone, 0, "", fmt.Errorf("update cc status: %w", err)
		}
		actionSuffix = auditActionWebhookSigningDeclinedSuffix
		notifyKind = NotifyKindDeclined
	default:
		actionSuffix = auditActionWebhookUnexpectedStatusSuffix
		notifyKind = NotifyKindNone
		fields := []any{
			"contract_id", contractRow.ID,
			"trustme_status_code_old", contractRow.TrustMeStatusCode,
			"trustme_status_code_new", ev.Status,
		}
		switch ev.Status {
		case 0, 2:
			s.logger.Info(ctx, "trustme webhook: intermediate status", fields...)
		default:
			s.logger.Warn(ctx, "trustme webhook: unexpected status", fields...)
		}
	}

	auditRepo := s.repoFactory.NewAuditRepo(tx)
	if err := s.recordAudit(ctx, auditRepo, actionSuffix, contractRow, view.CampaignCreatorID, ev); err != nil {
		return NotifyKindNone, 0, "", fmt.Errorf("audit: %w", err)
	}

	if view.CampaignIsDeleted {
		s.logger.Warn(ctx, "trustme webhook: webhook_for_deleted_campaign",
			"contract_id", contractRow.ID,
			"campaign_creator_id", view.CampaignCreatorID,
			"trustme_status_code_new", ev.Status,
		)
		return NotifyKindNone, 0, "", nil
	}

	return notifyKind, view.CreatorTelegramUserID, view.CampaignTmaURL, nil
}

// recordAudit пишет audit-row внутри той же Tx что и mutate. actor_id=NULL
// (system actor — webhook от TrustMe). Payload — UUID-only, без PII
// (security.md hard rule): contract_id, trustme_status_code_old/new.
func (s *WebhookService) recordAudit(
	ctx context.Context,
	auditRepo repository.AuditRepo,
	actionSuffix string,
	contractRow *repository.ContractRow,
	ccID string,
	ev domain.TrustMeWebhookEvent,
) error {
	payload := map[string]any{
		"contract_id":             contractRow.ID,
		"trustme_status_code_old": contractRow.TrustMeStatusCode,
		"trustme_status_code_new": ev.Status,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	entityID := ccID
	return auditRepo.Create(ctx, repository.AuditLogRow{
		ActorID:    nil,
		ActorRole:  auditActorRoleSystem,
		Action:     auditEntityTypeCampaignCreator + "." + actionSuffix,
		EntityType: auditEntityTypeCampaignCreator,
		EntityID:   &entityID,
		NewValue:   body,
	})
}
