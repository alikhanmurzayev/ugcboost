package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// HelloWorldReply is the placeholder text the skeleton bot sends back on
// any message. Real business logic will replace this once the higher-level
// chunks land.
const HelloWorldReply = "Hello, world!"

// Handler is the in-process business layer. Run() in transport calls
// Handle for every incoming update; the test-API endpoint calls it
// directly with a synthetic update + spy Sender.
type Handler struct{}

// NewHandler constructs the skeleton handler. Future chunks will accept
// dependencies (services, link repository, etc.) here.
func NewHandler() *Handler {
	return &Handler{}
}

// Handle processes one update. The skeleton replies "Hello, world!" to any
// text message and ignores everything else (channel posts, callback
// queries, edits — they will get real handling later).
func (h *Handler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	_, _ = sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   HelloWorldReply,
	})
}
