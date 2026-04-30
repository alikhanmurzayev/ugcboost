package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Sender is the minimal subset of *bot.Bot used by Handler. *bot.Bot
// satisfies it directly; tests inject a spy.
type Sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}
