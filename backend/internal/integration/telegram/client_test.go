package telegram_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestSpyClient_DrainIsAtomic(t *testing.T) {
	t.Parallel()

	t.Run("drain returns and clears outstanding messages", func(t *testing.T) {
		t.Parallel()
		spy := telegram.NewSpyClientForTest(logmocks.NewMockLogger(t))
		require.NoError(t, spy.SendMessage(context.Background(), 1, "hello"))
		require.NoError(t, spy.SendMessage(context.Background(), 1, "world"))

		got := spy.Drain(1)
		require.Equal(t, []telegram.SentMessage{
			{ChatID: 1, Text: "hello"},
			{ChatID: 1, Text: "world"},
		}, got)

		require.Empty(t, spy.Drain(1)) // drained, second drain is empty
	})

	t.Run("drain isolates by chat", func(t *testing.T) {
		t.Parallel()
		spy := telegram.NewSpyClientForTest(logmocks.NewMockLogger(t))
		require.NoError(t, spy.SendMessage(context.Background(), 1, "to-1"))
		require.NoError(t, spy.SendMessage(context.Background(), 2, "to-2"))

		require.Equal(t, []telegram.SentMessage{{ChatID: 1, Text: "to-1"}}, spy.Drain(1))
		require.Equal(t, []telegram.SentMessage{{ChatID: 2, Text: "to-2"}}, spy.Drain(2))
	})

	t.Run("concurrent sends preserve count", func(t *testing.T) {
		t.Parallel()
		spy := telegram.NewSpyClientForTest(logmocks.NewMockLogger(t))
		const goroutines = 16
		const perGoroutine = 32
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < perGoroutine; j++ {
					_ = spy.SendMessage(context.Background(), 42, "x")
				}
			}()
		}
		wg.Wait()
		require.Len(t, spy.Drain(42), goroutines*perGoroutine)
	})
}

func TestSpyClient_GetUpdatesIsEmpty(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{EnableTestEndpoints: true}
	client, _, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
	require.NoError(t, err)
	got, err := client.GetUpdates(context.Background(), 0, 0)
	require.NoError(t, err)
	require.Empty(t, got)
}
