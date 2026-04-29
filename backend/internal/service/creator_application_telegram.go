package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// telegramBotActorIP is the synthetic marker LinkTelegram stamps onto
// audit_logs.ip_address when there is no inbound HTTP request to derive a
// real client IP from. The column is NOT NULL — without an explicit value
// it would default to "", which lies about the operation.
const telegramBotActorIP = "telegram-bot"

// CreatorApplicationTelegramRepoFactory enumerates the repos the Telegram-link
// service needs. It is deliberately narrower than CreatorApplicationRepoFactory
// (Go convention: accept interfaces, return structs) so this service does not
// gain implicit access to repos it has no business calling.
type CreatorApplicationTelegramRepoFactory interface {
	NewCreatorApplicationRepo(db dbutil.DB) repository.CreatorApplicationRepo
	NewCreatorApplicationTelegramLinkRepo(db dbutil.DB) repository.CreatorApplicationTelegramLinkRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CreatorApplicationTelegramService binds a Telegram account to a creator
// application via the /start <applicationId> deep-link command. It owns the
// transactional write — application lookup, conflict detection, link insert
// and audit log — and exposes a single LinkTelegram entry point used by the
// bot's start handler.
type CreatorApplicationTelegramService struct {
	pool        dbutil.Pool
	repoFactory CreatorApplicationTelegramRepoFactory
	logger      logger.Logger
}

// NewCreatorApplicationTelegramService wires the service with its dependencies.
func NewCreatorApplicationTelegramService(pool dbutil.Pool, repoFactory CreatorApplicationTelegramRepoFactory, log logger.Logger) *CreatorApplicationTelegramService {
	return &CreatorApplicationTelegramService{pool: pool, repoFactory: repoFactory, logger: log}
}

// LinkTelegram performs the link operation atomically:
//
//  1. Trim and cap free-text Telegram metadata (username/first_name/last_name)
//     so a megabyte of attacker-controlled input cannot blow up DB rows.
//  2. Inside dbutil.WithTx:
//     2a. Fetch the application — sql.ErrNoRows propagates as domain.ErrNotFound;
//     a non-active status surfaces as TELEGRAM_APPLICATION_NOT_ACTIVE.
//     2b. Look up an existing link. Same Telegram user against same application
//     → idempotent success (no audit row written). Different Telegram user
//     → TELEGRAM_APPLICATION_ALREADY_LINKED.
//     2c. INSERT the link. UNIQUE conflict on telegram_user_id (account
//     already bound to another application) → TELEGRAM_ACCOUNT_ALREADY_LINKED.
//     PK conflict on application_id (race with another /start for the same
//     application) → re-read; idempotent vs ALREADY_LINKED. This closes the
//     preflight-SELECT vs INSERT window.
//     2d. Append an audit row with the four telegram_* fields in new_value.
//     audit_logs may carry PII per security.md.
//  3. After the transaction commits, log only stable identifiers
//     (application_id, telegram_user_id) — never username/first_name/last_name.
//     Idempotent links log nothing — the operation is a no-op for the audit
//     trail and stdout should not pretend a write happened.
func (s *CreatorApplicationTelegramService) LinkTelegram(ctx context.Context, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error) {
	username := capOptionalString(in.TgUsername, domain.TelegramUsernameMaxLen)
	firstName := capOptionalString(in.TgFirstName, domain.TelegramNameMaxLen)
	lastName := capOptionalString(in.TgLastName, domain.TelegramNameMaxLen)

	// audit_logs.ip_address is NOT NULL. The bot path has no HTTP middleware
	// so ClientIPFromContext would return "". Stamp a stable marker so the
	// audit row carries an honest source instead of an empty string.
	if middleware.ClientIPFromContext(ctx) == "" {
		ctx = middleware.WithClientIP(ctx, telegramBotActorIP)
	}

	var result *domain.TelegramLinkResult

	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		appRepo := s.repoFactory.NewCreatorApplicationRepo(tx)
		linkRepo := s.repoFactory.NewCreatorApplicationTelegramLinkRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		appRow, err := appRepo.GetByID(ctx, in.ApplicationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("get application: %w", err)
		}
		if !isLinkableStatus(appRow.Status) {
			// User-facing wording lives in telegram package's messages.go;
			// the BusinessError carries only the code, which the bot maps
			// to the canonical reply. Keeping the Message empty avoids two
			// drifting copies of the same text.
			return domain.NewBusinessError(domain.CodeTelegramApplicationNotActive, "")
		}

		existingLink, err := linkRepo.GetByApplicationID(ctx, in.ApplicationID)
		switch {
		case err == nil:
			// Application already has a link — idempotent vs conflict.
			if existingLink.TelegramUserID == in.TgUserID {
				result = &domain.TelegramLinkResult{
					ApplicationID:  appRow.ID,
					Status:         appRow.Status,
					TelegramUserID: existingLink.TelegramUserID,
					LinkedAt:       existingLink.LinkedAt,
					Idempotent:     true,
				}
				return nil
			}
			return domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "")
		case errors.Is(err, sql.ErrNoRows):
			// No link yet — proceed to insert.
		default:
			return fmt.Errorf("preflight telegram link: %w", err)
		}

		insertedLink, err := linkRepo.Insert(ctx, repository.CreatorApplicationTelegramLinkRow{
			ApplicationID:     appRow.ID,
			TelegramUserID:    in.TgUserID,
			TelegramUsername:  username,
			TelegramFirstName: firstName,
			TelegramLastName:  lastName,
			LinkedAt:          now,
		})
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrTelegramAccountLinkConflict):
				return domain.NewBusinessError(domain.CodeTelegramAccountAlreadyLinked, "")
			case errors.Is(err, domain.ErrTelegramApplicationLinkConflict):
				// PK race: another /start for this application slipped in
				// between our preflight SELECT and INSERT. Re-read to decide.
				raceLink, raceErr := linkRepo.GetByApplicationID(ctx, in.ApplicationID)
				if raceErr != nil {
					return fmt.Errorf("re-read telegram link after PK race: %w", raceErr)
				}
				if raceLink.TelegramUserID == in.TgUserID {
					result = &domain.TelegramLinkResult{
						ApplicationID:  appRow.ID,
						Status:         appRow.Status,
						TelegramUserID: raceLink.TelegramUserID,
						LinkedAt:       raceLink.LinkedAt,
						Idempotent:     true,
					}
					return nil
				}
				return domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "")
			default:
				return fmt.Errorf("insert telegram link: %w", err)
			}
		}

		auditPayload := map[string]any{
			"telegram_user_id":    insertedLink.TelegramUserID,
			"telegram_username":   insertedLink.TelegramUsername,
			"telegram_first_name": insertedLink.TelegramFirstName,
			"telegram_last_name":  insertedLink.TelegramLastName,
		}
		if err := writeAudit(ctx, auditRepo,
			AuditActionCreatorApplicationLinkTelegram, AuditEntityTypeCreatorApplication, appRow.ID,
			nil, auditPayload); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}

		result = &domain.TelegramLinkResult{
			ApplicationID:  appRow.ID,
			Status:         appRow.Status,
			TelegramUserID: insertedLink.TelegramUserID,
			LinkedAt:       insertedLink.LinkedAt,
			Idempotent:     false,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if result != nil && !result.Idempotent {
		// Identifiers only — Telegram username/first/last name are PII and stay
		// out of stdout per docs/standards/security.md.
		s.logger.Info(ctx, "telegram linked to creator application",
			"application_id", result.ApplicationID,
			"telegram_user_id", result.TelegramUserID)
	}
	return result, nil
}

// isLinkableStatus reports whether an application is in a state that allows
// the bot to bind a Telegram account. We allow the same "active" set used by
// the IIN uniqueness rule: pending / approved / blocked. Rejected applications
// are filtered out — the user already received a closure response and should
// open a new application instead of binding their Telegram to a dead row.
func isLinkableStatus(status string) bool {
	for _, allowed := range domain.CreatorApplicationActiveStatuses {
		if status == allowed {
			return true
		}
	}
	return false
}

// capOptionalString trims whitespace, drops empty results to nil and caps the
// remainder by rune count. Pointer-in / pointer-out so the SQL layer keeps
// nullable semantics intact.
func capOptionalString(s *string, max int) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	if rs := []rune(trimmed); len(rs) > max {
		trimmed = string(rs[:max])
	}
	return &trimmed
}
