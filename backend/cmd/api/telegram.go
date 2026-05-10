package main

import (
	"context"
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/closer"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// telegramRig bundles the Telegram-related dependencies main.go hands to the
// service layer and the test-API. Spy is non-nil iff EnableTestEndpoints is
// true — the test-API handler reads it directly to expose /test/telegram/sent.
// Notifier owns the outbound Sender, the WaitGroup the closer drains and the
// per-call timeout — services depend on a consumer-side notifier interface,
// never on the raw Sender or WaitGroup.
type telegramRig struct {
	Notifier *telegram.Notifier
	Spy      *telegram.SentSpyStore
}

// setupTelegram wires the outbound sender used by service-side notifications.
// Two independent switches drive the matrix:
//
//   - TelegramBotToken set → a real bot is constructed; outbound messages
//     reach actual Telegram users and the long-polling runner can consume
//     incoming updates.
//   - EnableTestEndpoints true → a SentSpyStore is created so the test-API
//     can replay incoming updates against the bot handler and read back the
//     replies.
//
// When both apply (staging) the sender is a Tee that fans out to the real
// API and the spy in parallel — real users receive the message AND e2e
// tests can inspect it. Token-only is production. Spy-only is local/CI where
// no token is configured. Neither is rejected at config load time outside
// production, but local/staging accept a missing token because the test-API
// keeps the bot surface usable from inside the test harness.
func setupTelegram(cfg *config.Config, log logger.Logger) (*telegramRig, error) {
	rig := &telegramRig{}
	if cfg.EnableTestEndpoints {
		rig.Spy = telegram.NewSentSpyStore()
	}

	var sender telegram.Sender
	switch {
	case cfg.TelegramBotToken != "":
		realBot, err := telegram.NewSendOnlyBot(cfg.TelegramBotToken)
		if err != nil {
			return nil, fmt.Errorf("create telegram send-only bot: %w", err)
		}
		if rig.Spy != nil {
			sender = telegram.NewTeeSender(realBot, rig.Spy)
		} else {
			sender = realBot
		}
	case rig.Spy != nil:
		sender = telegram.NewSpyOnlySender(rig.Spy)
	default:
		// config.Load already rejects this combo in production; if it slips
		// through (a future env added without updating the guard), surface it
		// here rather than handing the service layer a nil sender.
		return nil, fmt.Errorf("telegram: neither TELEGRAM_BOT_TOKEN nor EnableTestEndpoints configured")
	}
	rig.Notifier = telegram.NewNotifier(sender, log)
	return rig, nil
}

// startTelegramRunner spins up the long-polling goroutine whenever a real bot
// token is present. Local/CI runs without a token rely on the test-API to
// replay updates against the handler in-process; the runner is the only path
// that pulls updates from real Telegram, so its trigger is solely the token.
// The runner is registered in cl so SIGTERM cancels it before the pool closes.
func startTelegramRunner(ctx context.Context, cfg *config.Config, handler *telegram.Handler, log logger.Logger, cl *closer.Closer) {
	if cfg.TelegramBotToken == "" {
		log.Info(ctx, "telegram bot polling disabled", "token_set", false)
		return
	}
	runnerCtx, runnerCancel := context.WithCancel(context.Background())
	go telegram.Run(runnerCtx, cfg.TelegramBotToken, handler, log)
	cl.Add("telegram-runner", func(_ context.Context) error {
		runnerCancel()
		return nil
	})
	log.Info(ctx, "telegram bot polling enabled")
}

// registerNotifyWaiter adds a closer entry that waits for in-flight notify
// goroutines to finish. Bounded by the closer's parent context (shutdown
// timeout from cfg) — if a notify hangs past that, the wait is abandoned.
func registerNotifyWaiter(notifier *telegram.Notifier, cl *closer.Closer) {
	cl.Add("telegram-notify-wait", func(ctx context.Context) error {
		done := make(chan struct{})
		go func() {
			notifier.Wait()
			close(done)
		}()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
