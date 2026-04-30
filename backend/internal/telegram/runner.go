package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// restartBackoff is how long Run waits before recreating the bot when
// b.Start exits unexpectedly (panic in lib internals, fatal init error).
// 409 Conflict from a parallel deploy is handled INSIDE the library's
// getUpdates loop with its own exponential backoff (capped at 5s) — it
// never bubbles up here, so the process keeps the HTTP server alive and
// healthcheck stays green.
const restartBackoff = 10 * time.Second

// Run drives the Telegram long-polling client. It blocks until ctx is
// cancelled. If bot creation or Start exits non-cleanly it sleeps
// restartBackoff and tries again — losing one or two updates is acceptable
// (Telegram resends them after the next getUpdates).
func Run(ctx context.Context, token string, h *Handler, log logger.Logger) {
	for {
		err := runOnce(ctx, token, h, log)
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

func runOnce(ctx context.Context, token string, h *Handler, log logger.Logger) error {
	b, err := bot.New(token,
		bot.WithDefaultHandler(func(handlerCtx context.Context, b *bot.Bot, update *models.Update) {
			h.Handle(handlerCtx, b, update)
		}),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message"}),
		bot.WithErrorsHandler(func(err error) {
			// Library logs and backs off automatically; we surface it at
			// warn so operators see 401/409/timeouts but the process
			// stays alive (rolling deploy 409 is routine).
			log.Warn(ctx, "telegram api error", "error", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("telegram bot New: %w", err)
	}
	log.Info(ctx, "telegram bot started", "id", b.ID())
	b.Start(ctx) // blocks until ctx.Done
	return nil
}
