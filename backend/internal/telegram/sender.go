// Package telegram is the transport↔business seam for the Telegram bot.
//
// The library github.com/go-telegram/bot owns long polling, retries, panic
// recovery and HTTP transport. Our code only sees an *models.Update flowing
// in and a Sender flowing out. The Sender interface decouples the business
// handler from *bot.Bot — *bot.Bot satisfies it transparently in
// production, while tests inject a spy that records sent messages.
package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Sender is the minimal subset of *bot.Bot used by Handler. *bot.Bot
// already has the matching method, so passing it through requires no
// wrapper. Add new methods (SendPhoto, EditMessageText, …) here as the
// business layer grows.
type Sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}
