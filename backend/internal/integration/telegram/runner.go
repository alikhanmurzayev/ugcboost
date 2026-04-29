package telegram

import (
	"context"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// runnerRetryDelay is the fixed back-off the runner sleeps between failed
// getUpdates calls (Telegram 409, network blip, anything).
//
// Telegram's "terminated by other getUpdates request" surfaces during a
// Dokploy rolling deploy when both old and new containers temporarily compete
// for updates; the loser retries every 10s until SIGTERM ends the old
// container — measured deploy window is 10–60s. Increasing the delay
// stretches the recovery; decreasing it does not buy anything because the
// Bot API itself rate-limits.
const runnerRetryDelay = 10 * time.Second

// PollingRunner drives the long-polling loop. It owns the offset state and
// dispatches every update through the production dispatcher in the same
// goroutine, so update ordering is preserved and we move the offset only
// after Dispatch returns.
type PollingRunner struct {
	client       Client
	dispatcher   Dispatcher
	pollTimeout  time.Duration
	logger       logger.Logger
	done         chan struct{}
	retryDelay   time.Duration // overridable in tests
}

// NewPollingRunner wires the runner. pollTimeout maps directly to the
// long-poll timeout passed to getUpdates — usually cfg.TelegramPollingTimeout.
func NewPollingRunner(client Client, dispatcher Dispatcher, pollTimeout time.Duration, log logger.Logger) *PollingRunner {
	return &PollingRunner{
		client:      client,
		dispatcher:  dispatcher,
		pollTimeout: pollTimeout,
		logger:      log,
		done:        make(chan struct{}),
		retryDelay:  runnerRetryDelay,
	}
}

// Run is the long-polling loop. It returns nil when ctx is cancelled
// (graceful shutdown) — every error from getUpdates is treated as transient
// and retried after retryDelay. The done channel is closed on exit so Wait()
// can sync the closer.
//
// At-least-once delivery: the offset advances after dispatcher.Dispatch
// returns. Dispatch itself is synchronous and the link service is
// idempotent, so a re-delivered update produces the same outcome without an
// extra audit row.
func (r *PollingRunner) Run(ctx context.Context) error {
	defer close(r.done)

	var offset int64
	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := r.client.GetUpdates(ctx, offset, r.pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.logger.Warn(ctx, "telegram getUpdates failed, retrying", "error", err)
			select {
			case <-time.After(r.retryDelay):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		for _, u := range updates {
			r.dispatcher.Dispatch(ctx, u)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

// Wait blocks until Run has returned. The closer registers Wait so the
// shutdown sequence does not exit while the runner goroutine is still in
// flight.
func (r *PollingRunner) Wait() {
	<-r.done
}
