package telegram_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestPollingRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("dispatches updates and advances offset", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())

		var dispatched atomic.Int32
		dispatcher.EXPECT().Dispatch(mock.Anything, mock.AnythingOfType("telegram.IncomingUpdate")).
			Run(func(_ context.Context, _ telegram.IncomingUpdate) {
				if dispatched.Add(1) == 2 {
					cancel()
				}
			})

		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Return([]telegram.IncomingUpdate{
				{UpdateID: 5, ChatID: 1, Text: "/start"},
				{UpdateID: 7, ChatID: 1, Text: "/help"},
			}, nil).Once()
		client.EXPECT().GetUpdates(mock.Anything, int64(8), 30*time.Second).
			Return(nil, context.Canceled).Maybe()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(10 * time.Millisecond)
		require.NoError(t, runner.Run(ctx))
		runner.Wait()
	})

	t.Run("retries after error, exits on cancellation during sleep", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())

		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Return(nil, errors.New("409 conflict")).Once()
		log.EXPECT().Warn(mock.Anything, "telegram getUpdates failed, retrying",
			mock.Anything).Once()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(5 * time.Second)

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		require.NoError(t, runner.Run(ctx))
		require.Less(t, time.Since(start), 1*time.Second, "runner should exit promptly via ctx.Done")
		runner.Wait()
	})

	t.Run("ctx cancelled before first GetUpdates returns immediately", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		require.NoError(t, runner.Run(ctx))
	})

	t.Run("ctx cancelled while GetUpdates returns ctx-error skips warn", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())

		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Run(func(_ context.Context, _ int64, _ time.Duration) {
				cancel()
			}).
			Return(nil, context.Canceled).Once()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(1 * time.Millisecond)
		require.NoError(t, runner.Run(ctx))
	})

	t.Run("safeDispatch recovers from dispatcher panic, advances offset, warns user-may-need-retry", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())

		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Return([]telegram.IncomingUpdate{
				{UpdateID: 1, ChatID: 1, Text: "/start abc"},
			}, nil).Once()
		dispatcher.EXPECT().Dispatch(mock.Anything, mock.AnythingOfType("telegram.IncomingUpdate")).
			Run(func(_ context.Context, _ telegram.IncomingUpdate) {
				panic("boom from handler")
			})
		log.EXPECT().Error(mock.Anything, "telegram dispatcher panic recovered", mock.Anything).Once()
		log.EXPECT().Warn(mock.Anything,
			"telegram update advanced past panicked handler — user may need to retry /start",
			mock.Anything).Once()
		client.EXPECT().GetUpdates(mock.Anything, int64(2), 30*time.Second).
			Run(func(_ context.Context, _ int64, _ time.Duration) {
				cancel() // exit the loop after the panic was absorbed
			}).
			Return(nil, context.Canceled).Maybe()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(1 * time.Millisecond)
		require.NoError(t, runner.Run(ctx))
	})

	t.Run("Bot API retry-after honoured over fixed delay", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)
		ctx, cancel := context.WithCancel(context.Background())

		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Return(nil, telegram.NewAPIErrorForTest(429, "Too Many Requests", 50*time.Millisecond)).Once()
		log.EXPECT().Warn(mock.Anything, "telegram getUpdates failed, retrying", mock.Anything).Once()
		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Run(func(_ context.Context, _ int64, _ time.Duration) {
				cancel()
			}).
			Return(nil, context.Canceled).Once()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(5 * time.Second) // fixed delay would block far longer
		start := time.Now()
		require.NoError(t, runner.Run(ctx))
		require.Less(t, time.Since(start), 1*time.Second,
			"runner should follow Telegram's retry-after, not the fixed delay")
	})

	t.Run("Wait actually blocks until Run returns", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		dispatcher := mocks.NewMockDispatcher(t)
		log := logmocks.NewMockLogger(t)

		// Long-running GetUpdates: block until ctx is cancelled, so Run
		// stays inside the loop until we cancel below. This way Wait()
		// genuinely has to block instead of trivially returning because
		// Run already exited.
		ctx, cancel := context.WithCancel(context.Background())
		client.EXPECT().GetUpdates(mock.Anything, int64(0), 30*time.Second).
			Run(func(getCtx context.Context, _ int64, _ time.Duration) {
				<-getCtx.Done()
			}).
			Return(nil, context.Canceled).Maybe()

		runner := telegram.NewPollingRunner(client, dispatcher, 30*time.Second, log)
		runner.SetRetryDelayForTest(1 * time.Millisecond)

		runDone := make(chan struct{})
		go func() {
			_ = runner.Run(ctx)
			close(runDone)
		}()

		waitDone := make(chan struct{})
		go func() {
			runner.Wait()
			close(waitDone)
		}()

		// Before cancel: Wait must still be blocked.
		select {
		case <-waitDone:
			t.Fatal("Wait returned before Run was cancelled")
		case <-time.After(50 * time.Millisecond):
		}
		cancel()

		// After cancel: Wait must unblock promptly.
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			t.Fatal("Wait did not unblock after Run finished")
		}
		<-runDone
	})
}
