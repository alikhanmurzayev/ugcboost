package telegram

import (
	"context"
	"errors"
	"fmt"
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

// PollingRunner drives the long-polling loop. Single-shot — Run must be
// called exactly once per instance; the `done` channel is created in the
// constructor and closed on exit. The runner owns the offset state and
// dispatches every update through the production dispatcher in the same
// goroutine, so update ordering is preserved and we move the offset only
// after Dispatch returns.
type PollingRunner struct {
	client      Client
	dispatcher  Dispatcher
	pollTimeout time.Duration
	logger      logger.Logger
	done        chan struct{}
	retryDelay  time.Duration // overridable in tests
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
// extra audit row. Any future command added to the dispatcher is REQUIRED
// to be idempotent for the same reason.
//
// Each Dispatch is wrapped in a recover() so a panic in one handler cannot
// kill the whole runner goroutine — the bot would otherwise go silent until
// the process restarts.
//
// On a Bot API error carrying `parameters.retry_after`, the runner honours
// that delay instead of the fixed retryDelay (e.g. 429 Too Many Requests).
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
			delay := r.retryDelay
			var apiErr *telegramAPIError
			if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		for _, u := range updates {
			if ctx.Err() != nil {
				return nil
			}
			panicked := r.safeDispatch(ctx, u)
			// Advance offset even after a recovered panic. Telegram's
			// at-least-once contract + the link service's idempotency cover
			// re-delivery, but a handler-side panic can leave the user
			// without a reply. We surface that explicitly so a crashed
			// handler is loud in stdout instead of being silently absorbed.
			if panicked {
				r.logger.Warn(ctx, "telegram update advanced past panicked handler — user may need to retry /start",
					"update_id", u.UpdateID)
			}
			// Monotonic offset: skip non-monotonic / equal ids (Bot API
			// promises monotonicity, defensive-only).
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

// safeDispatch wraps a single Dispatch with recover so a panic in any
// downstream handler is logged and skipped instead of bringing down the
// whole runner goroutine — without recover() one bug in start_handler /
// future commands would silence the bot until process restart, with no
// audit trail of what failed. Returns true when a panic was recovered so
// the caller can surface a follow-up warning.
//
// The recovered value is stringified before logging so a panic carrying a
// struct/credential reference cannot leak its content into stdout via
// slog's default attr serialisation. Only the textual representation
// (fmt.Sprintf %v, capped) and the type are kept.
func (r *PollingRunner) safeDispatch(ctx context.Context, u IncomingUpdate) (panicked bool) {
	defer func() {
		if rec := recover(); rec != nil {
			panicked = true
			panicSummary := fmt.Sprintf("%v", rec)
			if len(panicSummary) > 256 {
				panicSummary = panicSummary[:256]
			}
			r.logger.Error(ctx, "telegram dispatcher panic recovered",
				"update_id", u.UpdateID,
				"panic_summary", panicSummary)
		}
	}()
	r.dispatcher.Dispatch(ctx, u)
	return false
}

// Wait blocks until Run has returned. The closer registers Wait so the
// shutdown sequence does not exit while the runner goroutine is still in
// flight.
func (r *PollingRunner) Wait() {
	<-r.done
}
