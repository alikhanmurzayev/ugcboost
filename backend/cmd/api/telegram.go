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

// setupTelegram builds the Sender per the (TelegramMock, EnableTestEndpoints)
// matrix and wraps it in a Notifier. The four resulting modes are: spy-only
// (mock=true, anywhere), real+spy Tee (mock=false + test endpoints, i.e.
// staging), real flat (mock=false + production), and the impossible
// (mock=true + production would be configurable but pointless — production
// never has test endpoints so the spy would dangle; we still build a
// SpyOnlySender so the service layer always has a working Sender).
func setupTelegram(cfg *config.Config, log logger.Logger) (*telegramRig, error) {
	rig := &telegramRig{}
	if cfg.EnableTestEndpoints {
		rig.Spy = telegram.NewSentSpyStore()
	}

	var sender telegram.Sender
	if cfg.TelegramMock {
		if rig.Spy == nil {
			rig.Spy = telegram.NewSentSpyStore()
		}
		sender = telegram.NewSpyOnlySender(rig.Spy)
	} else {
		realBot, err := telegram.NewSendOnlyBot(cfg.TelegramBotToken)
		if err != nil {
			return nil, fmt.Errorf("create telegram send-only bot: %w", err)
		}
		if rig.Spy != nil {
			sender = telegram.NewTeeSender(realBot, rig.Spy)
		} else {
			sender = realBot
		}
	}
	rig.Notifier = telegram.NewNotifier(sender, log)
	return rig, nil
}

// startTelegramRunner spins up the long-polling goroutine when a real bot
// token is configured AND mock mode is off. Mock mode skips polling because
// no real bot would respond. The runner is registered in cl so SIGTERM
// cancels it before the pool closes.
func startTelegramRunner(ctx context.Context, cfg *config.Config, handler *telegram.Handler, log logger.Logger, cl *closer.Closer) {
	if cfg.TelegramMock || cfg.TelegramBotToken == "" {
		log.Info(ctx, "telegram bot polling disabled", "mock", cfg.TelegramMock, "token_set", cfg.TelegramBotToken != "")
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
