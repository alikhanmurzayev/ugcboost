package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Sender is the minimal subset of *bot.Bot used by Handler and the outbound
// notification helpers. *bot.Bot satisfies it directly; tests inject a spy.
type Sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}

// NewSendOnlyBot creates a *bot.Bot configured purely for outbound API
// calls — no default handler, no polling. The instance is safe to use as a
// Sender from any service that needs to push a message without consuming
// updates. Telegram allows multiple concurrent senders for the same token,
// so this coexists with the long-polling bot from Run.
func NewSendOnlyBot(token string) (*bot.Bot, error) {
	return bot.New(token)
}
