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
func (h *StartHandler) Handle(ctx context.Context, update IncomingUpdate, payload string) {
	appID, err := uuid.Parse(payload)
	if err != nil {
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
