package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/closer"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// telegramRig bundles the Telegram-related dependencies main.go hands to the
// service layer and the test-API. Spy is non-nil iff EnableTestEndpoints is
// true. NotifyWG tracks fire-and-forget notify goroutines so the closer can
// wait for them to drain before the pool shuts down.
type telegramRig struct {
	Sender   telegram.Sender
	Spy      *telegram.SentSpyStore
	NotifyWG *sync.WaitGroup
}

// setupTelegram builds the Sender per the (TelegramMock, EnableTestEndpoints)
// matrix. The four resulting modes are: spy-only (mock=true, anywhere),
// real+spy Tee (mock=false + test endpoints, i.e. staging), real flat
// (mock=false + production), and the impossible (mock=true + production
// would be configurable but pointless — production never has test endpoints
// so the spy would dangle; we still build a SpyOnlySender so the service
// layer always has a working Sender).
func setupTelegram(cfg *config.Config, log logger.Logger) (*telegramRig, error) {
	rig := &telegramRig{NotifyWG: &sync.WaitGroup{}}
	if cfg.EnableTestEndpoints {
		rig.Spy = telegram.NewSentSpyStore()
	}

	if cfg.TelegramMock {
		if rig.Spy == nil {
			rig.Spy = telegram.NewSentSpyStore()
		}
		rig.Sender = telegram.NewSpyOnlySender(rig.Spy)
		return rig, nil
	}

	realBot, err := telegram.NewSendOnlyBot(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram send-only bot: %w", err)
	}
	if rig.Spy != nil {
		rig.Sender = telegram.NewTeeSender(realBot, rig.Spy)
	} else {
		rig.Sender = realBot
	}
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
func registerNotifyWaiter(wg *sync.WaitGroup, cl *closer.Closer) {
	cl.Add("telegram-notify-wait", func(ctx context.Context) error {
		done := make(chan struct{})
		go func() {
			wg.Wait()
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
