package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

const restartBackoff = 10 * time.Second

// Run drives the long-polling client. Blocks until ctx is cancelled.
// 409 Conflict from a parallel deploy is handled inside the library's
// getUpdates loop (own exponential backoff up to 5s); the outer loop
// here only kicks in if bot.New or b.Start dies for some other reason.
func Run(ctx context.Context, token string, h *Handler, log logger.Logger) {
	for {
		err := startBot(ctx, token, h, log)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Error(ctx, "telegram bot init failed", "error", err)
		} else {
			log.Warn(ctx, "telegram polling stopped, restarting")
		}
		select {
		case <-time.After(restartBackoff):
		case <-ctx.Done():
			return
		}
	}
}

func startBot(ctx context.Context, token string, h *Handler, log logger.Logger) error {
	b, err := bot.New(token,
		bot.WithDefaultHandler(func(handlerCtx context.Context, b *bot.Bot, update *models.Update) {
			h.Handle(handlerCtx, b, update)
		}),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message"}),
		bot.WithErrorsHandler(func(err error) {
			log.Warn(ctx, "telegram api error", "error", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("telegram bot New: %w", err)
	}
	log.Info(ctx, "telegram bot started", "id", b.ID())
	b.Start(ctx)
	return nil
}
