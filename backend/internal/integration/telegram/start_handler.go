package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// LinkService is the narrow service contract the start handler depends on —
// just enough to bind a Telegram account to a creator application. Defined
// here so the bot package owns its dependency surface (accept interfaces).
type LinkService interface {
	LinkTelegram(ctx context.Context, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error)
}

// nowFunc is the clock the start handler stamps every link with. Production
// passes time.Now (UTC enforced inside the handler) — tests can swap a fixed
// clock without monkey-patching the package.
type nowFunc func() time.Time

// StartHandler responds to /start <payload>. It parses the UUID payload,
// shells out to the link service, then maps the return value (or domain
// error) to one of the Russian copy lines from messages.go.
type StartHandler struct {
	service  LinkService
	client   Client
	messages Messages
	logger   logger.Logger
	now      nowFunc
}

// NewStartHandler wires the handler with its dependencies. The clock defaults
// to time.Now in UTC so the handler stays stable across timezones.
func NewStartHandler(service LinkService, client Client, messages Messages, log logger.Logger) *StartHandler {
	return &StartHandler{
		service:  service,
		client:   client,
		messages: messages,
		logger:   log,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Handle implements startCommandHandler. The dispatcher calls it with the
// trimmed payload (everything after "/start "). Empty or whitespace-only
// payloads are caught upstream by the dispatcher and never reach here.
//
// Payload validation is deliberately strict: the deep-link Telegram opens is
// always `?start=<canonical-uuid-form>` (8-4-4-4-12 hex with dashes). The
// shape check up front rejects looser variants (`urn:uuid:...`, braces,
// no-dashes) that uuid.Parse would otherwise accept — once we're past the
// shape check, uuid.Parse always succeeds on a 36-char hex+dash string, so
// the only remaining failure mode is the nil UUID sentinel.
func (h *StartHandler) Handle(ctx context.Context, update IncomingUpdate, payload string) {
	if !looksLikeCanonicalUUID(payload) {
		h.reply(ctx, update.ChatID, h.messages.InvalidPayload())
		return
	}
	appID, err := uuid.Parse(payload)
	if err != nil {
		// Defensive: shape-check guarantees uuid.Parse succeeds, but
		// keeping the branch protects against a future library upgrade
		// changing semantics.
		h.reply(ctx, update.ChatID, h.messages.InvalidPayload())
		return
	}
	if appID == uuid.Nil {
		h.reply(ctx, update.ChatID, h.messages.InvalidPayload())
		return
	}

	in := domain.TelegramLinkInput{
		ApplicationID: appID.String(),
		TgUserID:      update.UserID,
		TgUsername:    update.Username,
		TgFirstName:   update.FirstName,
		TgLastName:    update.LastName,
	}
	if _, err := h.service.LinkTelegram(ctx, in, h.now()); err != nil {
		h.reply(ctx, update.ChatID, h.errorReply(ctx, err))
		return
	}
	h.reply(ctx, update.ChatID, h.messages.LinkSuccess())
}

// errorReply maps a service error to the matching Russian copy line. Unknown
// errors degrade to InternalError and are logged so we can investigate; only
// stable identifiers (chat_id, application_id) reach stdout — no PII.
func (h *StartHandler) errorReply(ctx context.Context, err error) string {
	if errors.Is(err, domain.ErrNotFound) {
		return h.messages.ApplicationNotFound()
	}
	var be *domain.BusinessError
	if errors.As(err, &be) {
		switch be.Code {
		case domain.CodeTelegramApplicationNotActive:
			return h.messages.ApplicationNotActive()
		case domain.CodeTelegramApplicationAlreadyLinked:
			return h.messages.ApplicationAlreadyLinked()
		case domain.CodeTelegramAccountAlreadyLinked:
			return h.messages.AccountAlreadyLinked()
		}
	}
	h.logger.Error(ctx, "telegram start handler internal error", "error", err)
	return h.messages.InternalError()
}

func (h *StartHandler) reply(ctx context.Context, chatID int64, text string) {
	if err := h.client.SendMessage(ctx, chatID, text); err != nil {
		h.logger.Warn(ctx, "telegram start handler reply failed", "chat_id", chatID, "error", err)
	}
}

// looksLikeCanonicalUUID is a fast string-shape check before uuid.Parse:
// length 36, dashes at positions 8/13/18/23, hex digits everywhere else. It
// rejects `urn:uuid:` prefixes, braces, and the dash-less form that
// `uuid.Parse` accepts liberally. The downstream uuid.Parse still validates
// the hex characters definitively.
func looksLikeCanonicalUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexDigit(r) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f':
		return true
	case r >= 'A' && r <= 'F':
		return true
	}
	return false
}
