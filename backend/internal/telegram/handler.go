package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

type Handler struct {
	log logger.Logger
}

func NewHandler(log logger.Logger) *Handler {
	return &Handler{log: log}
}

func (h *Handler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Hello, world!",
	}); err != nil {
		h.log.Error(ctx, "telegram send message failed", "error", err, "chat_id", update.Message.Chat.ID)
	}
}
