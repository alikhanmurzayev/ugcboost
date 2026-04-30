package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// LinkService is the narrow contract the handler depends on — only the
// /start binding entry point. Defined here so the bot package owns its
// dependency surface (accept interfaces, return structs).
type LinkService interface {
	LinkTelegram(ctx context.Context, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error)
}

// Handler routes every incoming update through one entry point.
type Handler struct {
	link LinkService
	log  logger.Logger
	now  func() time.Time
}

// NewHandler wires the handler. The clock defaults to time.Now in UTC.
func NewHandler(link LinkService, log logger.Logger) *Handler {
	return &Handler{
		link: link,
		log:  log,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Handle is the single dispatcher invoked by the long-poll runner. It
// understands one command — /start <uuid> — and falls back to the generic
// "submit on the website" reply for everything else.
func (h *Handler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	payload, isStart := startPayload(text)
	if !isStart {
		h.reply(ctx, sender, chatID, MessageFallback)
		return
	}
	if payload == "" || !looksLikeCanonicalUUID(payload) {
		h.reply(ctx, sender, chatID, MessageFallback)
		return
	}
	appID, err := uuid.Parse(payload)
	if err != nil || appID == uuid.Nil {
		h.reply(ctx, sender, chatID, MessageFallback)
		return
	}

	in := buildLinkInput(appID, update.Message.From)
	if _, err := h.link.LinkTelegram(ctx, in, h.now()); err != nil {
		h.reply(ctx, sender, chatID, h.errorReply(ctx, err))
		return
	}
	h.reply(ctx, sender, chatID, MessageLinkSuccess)
}

// startPayload returns (payload, true) for "/start" / "/start <something>"
// and ("", false) for any other text. The payload is whatever follows the
// command, with surrounding whitespace trimmed.
func startPayload(text string) (string, bool) {
	const cmd = "/start"
	switch {
	case text == cmd:
		return "", true
	case strings.HasPrefix(text, cmd+" "):
		return strings.TrimSpace(text[len(cmd)+1:]), true
	default:
		return "", false
	}
}

func buildLinkInput(appID uuid.UUID, from *models.User) domain.TelegramLinkInput {
	in := domain.TelegramLinkInput{ApplicationID: appID.String()}
	if from == nil {
		return in
	}
	in.TgUserID = from.ID
	if u := from.Username; u != "" {
		in.TgUsername = &u
	}
	if f := from.FirstName; f != "" {
		in.TgFirstName = &f
	}
	if l := from.LastName; l != "" {
		in.TgLastName = &l
	}
	return in
}

// errorReply maps a service error to the matching reply. Unknown errors
// degrade to the internal-error copy and are logged so we can investigate;
// only stable identifiers reach stdout — no PII per security.md.
func (h *Handler) errorReply(ctx context.Context, err error) string {
	if errors.Is(err, domain.ErrNotFound) {
		return MessageApplicationNotFound
	}
	var be *domain.BusinessError
	if errors.As(err, &be) && be.Code == domain.CodeTelegramApplicationAlreadyLinked {
		return MessageApplicationAlreadyLinked
	}
	h.log.Error(ctx, "telegram link failed", "error", err)
	return MessageInternalError
}

func (h *Handler) reply(ctx context.Context, sender Sender, chatID int64, text string) {
	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
		h.log.Error(ctx, "telegram send message failed", "error", err, "chat_id", chatID)
	}
}

// looksLikeCanonicalUUID enforces 8-4-4-4-12 hex-with-dashes shape so loose
// forms (urn:uuid:..., {...}, no dashes) that uuid.Parse accepts are rejected
// up front. Telegram deep-link payloads are always the canonical form.
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
