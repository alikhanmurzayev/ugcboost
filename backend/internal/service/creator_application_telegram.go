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

// telegramBotActorIP is the synthetic marker the LinkTelegram path stamps on
// audit_logs.ip_address — the bot has no notion of a Telegram-side client IP.
const telegramBotActorIP = "telegram-bot"

// CreatorApplicationTelegramRepoFactory is a narrow set of repos used by the
// link service.
type CreatorApplicationTelegramRepoFactory interface {
	NewCreatorApplicationRepo(db dbutil.DB) repository.CreatorApplicationRepo
	NewCreatorApplicationTelegramLinkRepo(db dbutil.DB) repository.CreatorApplicationTelegramLinkRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// CreatorApplicationTelegramService binds a Telegram account to a creator
// application via the /start <applicationId> deep-link command.
type CreatorApplicationTelegramService struct {
	pool        dbutil.Pool
	repoFactory CreatorApplicationTelegramRepoFactory
	logger      logger.Logger
}

// NewCreatorApplicationTelegramService wires the service.
func NewCreatorApplicationTelegramService(pool dbutil.Pool, repoFactory CreatorApplicationTelegramRepoFactory, log logger.Logger) *CreatorApplicationTelegramService {
	return &CreatorApplicationTelegramService{pool: pool, repoFactory: repoFactory, logger: log}
}

// LinkTelegram performs the link operation atomically. Behaviour mirrors the
// rules agreed for the bot:
//
//   - Application must exist; otherwise domain.ErrNotFound.
//   - Application of any status (including rejected) accepts a link.
//   - One application binds to at most one Telegram account (PK guards this).
//     A repeat /start from the same TG is idempotent (no audit, no log);
//     a different TG hitting an already-linked application yields
//     CodeTelegramApplicationAlreadyLinked.
//   - One TG may be linked to many applications (no UNIQUE on telegram_user_id).
func (s *CreatorApplicationTelegramService) LinkTelegram(ctx context.Context, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error) {
	username := capOptionalString(in.TgUsername, domain.TelegramUsernameMaxLen)
	firstName := capOptionalString(in.TgFirstName, domain.TelegramNameMaxLen)
	lastName := capOptionalString(in.TgLastName, domain.TelegramNameMaxLen)

	ctx = middleware.WithClientIP(ctx, telegramBotActorIP)

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

		existingLink, err := linkRepo.GetByApplicationID(ctx, in.ApplicationID)
		switch {
		case err == nil:
			if existingLink.TelegramUserID == in.TgUserID {
				result = idempotentResult(appRow.Status, existingLink)
				return nil
			}
			return domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "")
		case errors.Is(err, sql.ErrNoRows):
			// not linked yet — proceed to insert
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
			if !errors.Is(err, domain.ErrTelegramApplicationLinkConflict) {
				return fmt.Errorf("insert telegram link: %w", err)
			}
			// PK race: another /start for this application slipped in between
			// our preflight SELECT and INSERT. Re-read to decide.
			raceLink, raceErr := linkRepo.GetByApplicationID(ctx, in.ApplicationID)
			if raceErr != nil {
				return fmt.Errorf("re-read telegram link after PK race: %w", raceErr)
			}
			if raceLink.TelegramUserID == in.TgUserID {
				result = idempotentResult(appRow.Status, raceLink)
				return nil
			}
			return domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "")
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
		// Identifiers only — Telegram username/first/last name stay out of
		// stdout per docs/standards/security.md.
		s.logger.Info(ctx, "telegram linked to creator application",
			"application_id", result.ApplicationID,
			"telegram_user_id", result.TelegramUserID)
	}
	return result, nil
}

func idempotentResult(status string, row *repository.CreatorApplicationTelegramLinkRow) *domain.TelegramLinkResult {
	return &domain.TelegramLinkResult{
		ApplicationID:  row.ApplicationID,
		Status:         status,
		TelegramUserID: row.TelegramUserID,
		LinkedAt:       row.LinkedAt,
		Idempotent:     true,
	}
}

// capOptionalString trims whitespace, drops empty results to nil and caps the
// remainder by rune count so attacker-controlled mega-strings cannot blow up
// DB rows.
func capOptionalString(s *string, maxLen int) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	if rs := []rune(trimmed); len(rs) > maxLen {
		trimmed = string(rs[:maxLen])
	}
	return &trimmed
}
