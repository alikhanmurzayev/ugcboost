package telegram

import (
	"context"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// Dispatcher routes a single IncomingUpdate to the right handler. Today the
// only command is /start <uuid>; everything else goes to a fallback reply.
// Future commands (e.g. /status) plug in here without touching the runner.
type Dispatcher interface {
	Dispatch(ctx context.Context, update IncomingUpdate)
}

// StartCommandHandler is the narrow contract dispatcher needs to reach the
// /start handler — accept-interfaces / return-structs.
type StartCommandHandler interface {
	Handle(ctx context.Context, update IncomingUpdate, payload string)
}

// dispatcher implements Dispatcher. Send-only failures (e.g. SendMessage
// returning a Telegram-side error) are logged here once and then swallowed —
// the runner's offset advance after Dispatch makes one missed reply the price
// of forward progress; the user can re-issue /start to recover.
type dispatcher struct {
	client   Client
	start    StartCommandHandler
	messages Messages
	logger   logger.Logger
}

// NewDispatcher wires the dispatcher with its dependencies.
func NewDispatcher(client Client, start StartCommandHandler, messages Messages, log logger.Logger) Dispatcher {
	return &dispatcher{client: client, start: start, messages: messages, logger: log}
}

func (d *dispatcher) Dispatch(ctx context.Context, update IncomingUpdate) {
	text := strings.TrimSpace(update.Text)

	switch {
	case text == "/start":
		d.reply(ctx, update.ChatID, d.messages.StartNoPayload())
	case strings.HasPrefix(text, "/start "):
		payload := strings.TrimSpace(strings.TrimPrefix(text, "/start "))
		d.start.Handle(ctx, update, payload)
	default:
		d.reply(ctx, update.ChatID, d.messages.Fallback())
	}
}

func (d *dispatcher) reply(ctx context.Context, chatID int64, text string) {
	if err := d.client.SendMessage(ctx, chatID, text); err != nil {
		// chatID is a Telegram identifier (numeric, public-ish), not PII per
		// security.md, so logging it here is safe. Reply text is templated
		// copy from messages.go — also non-PII.
		d.logger.Warn(ctx, "telegram dispatcher reply failed", "chat_id", chatID, "error", err)
	}
}
