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
//   - Application must exist; otherwise domain.ErrNotFound (FK violation on
//     INSERT also maps here — handles the race where the parent row is gone
//     between the appRepo.GetByID preflight and our insert).
//   - Application of any status (including rejected) accepts a link.
//   - One application binds to at most one Telegram account (PK guards this).
//     A repeat /start from the same TG is idempotent (no audit, debug-log only);
//     a different TG hitting an already-linked application yields
//     CodeTelegramApplicationAlreadyLinked.
//   - One TG may be linked to many applications (no UNIQUE on telegram_user_id).
func (s *CreatorApplicationTelegramService) LinkTelegram(ctx context.Context, in domain.TelegramLinkInput, now time.Time) error {
	username := capOptionalString(in.TelegramUsername, domain.TelegramUsernameMaxLen)
	firstName := capOptionalString(in.TelegramFirstName, domain.TelegramNameMaxLen)
	lastName := capOptionalString(in.TelegramLastName, domain.TelegramNameMaxLen)

	ctx = middleware.WithClientIP(ctx, telegramBotActorIP)

	var idempotent bool

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

		// Preflight read of the link row. Postgres aborts the whole
		// transaction on a constraint violation, so we cannot rely on
		// catching 23505 from INSERT and then re-reading in the same tx
		// (the SELECT would also fail with "current transaction is
		// aborted"). The 23505 branch below is kept only as a safety net
		// for the genuine race window between this preflight and the
		// INSERT — rare but not impossible under concurrent /start.
		existingLink, err := linkRepo.GetByApplicationID(ctx, in.ApplicationID)
		switch {
		case err == nil:
			if existingLink.TelegramUserID == in.TelegramUserID {
				idempotent = true
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
			TelegramUserID:    in.TelegramUserID,
			TelegramUsername:  username,
			TelegramFirstName: firstName,
			TelegramLastName:  lastName,
			LinkedAt:          now,
		})
		if err != nil {
			if errors.Is(err, domain.ErrTelegramApplicationLinkConflict) {
				// PK race: another /start for this application slipped in
				// between our preflight SELECT and INSERT. Both decisions
				// (idempotent vs already-linked) collapse onto the data
				// the winning transaction wrote — at this point our own
				// tx is aborted, so we surface a generic business error.
				// The next /start from this user will succeed via the
				// preflight path above.
				return domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "")
			}
			// FK violation (parent application gone) is already mapped to
			// domain.ErrNotFound by the repo; everything else bubbles up.
			return fmt.Errorf("insert telegram link: %w", err)
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
		return nil
	})
	if err != nil {
		return err
	}

	// Identifiers only — Telegram username/first/last name stay out of stdout
	// per docs/standards/security.md.
	if idempotent {
		s.logger.Debug(ctx, "telegram link idempotent",
			"application_id", in.ApplicationID,
			"telegram_user_id", in.TelegramUserID)
		return nil
	}
	s.logger.Info(ctx, "telegram linked to creator application",
		"application_id", in.ApplicationID,
		"telegram_user_id", in.TelegramUserID)
	return nil
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
