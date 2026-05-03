package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"

	mocks "github.com/alikhanmurzayev/ugcboost/backend/internal/telegram/mocks"
)

func TestSpyOnlySender_SendMessage(t *testing.T) {
	t.Parallel()

	t.Run("records and returns synthetic message", func(t *testing.T) {
		t.Parallel()
		store := NewSentSpyStore()
		s := NewSpyOnlySender(store)

		msg, err := s.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: int64(7), Text: "hi"})
		require.NoError(t, err)
		require.NotNil(t, msg)
		require.Equal(t, 1, msg.ID)

		got := store.List(SentFilter{})
		require.Len(t, got, 1)
		require.Equal(t, int64(7), got[0].ChatID)
		require.Equal(t, "hi", got[0].Text)
	})

	t.Run("nil store panics", func(t *testing.T) {
		t.Parallel()
		require.Panics(t, func() { NewSpyOnlySender(nil) })
	})
}

func TestTeeSender_SendMessage(t *testing.T) {
	t.Parallel()

	t.Run("delegates to real and records on success", func(t *testing.T) {
		t.Parallel()
		real := mocks.NewMockSender(t)
		real.EXPECT().SendMessage(context.Background(), &bot.SendMessageParams{ChatID: int64(9), Text: "ok"}).
			Return(&models.Message{ID: 1234}, nil)

		store := NewSentSpyStore()
		tee := NewTeeSender(real, store)

		msg, err := tee.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: int64(9), Text: "ok"})
		require.NoError(t, err)
		require.Equal(t, 1234, msg.ID)
		got := store.List(SentFilter{})
		require.Len(t, got, 1)
		require.Empty(t, got[0].Err)
	})

	t.Run("records error from real but propagates it", func(t *testing.T) {
		t.Parallel()
		real := mocks.NewMockSender(t)
		boom := errors.New("upstream down")
		real.EXPECT().SendMessage(context.Background(), &bot.SendMessageParams{ChatID: int64(1)}).
			Return(nil, boom)

		store := NewSentSpyStore()
		tee := NewTeeSender(real, store)

		_, err := tee.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: int64(1)})
		require.ErrorIs(t, err, boom)

		got := store.List(SentFilter{})
		require.Len(t, got, 1)
		require.Equal(t, "upstream down", got[0].Err)
	})

	t.Run("nil real panics", func(t *testing.T) {
		t.Parallel()
		require.Panics(t, func() { NewTeeSender(nil, NewSentSpyStore()) })
	})

	t.Run("nil store panics", func(t *testing.T) {
		t.Parallel()
		real := mocks.NewMockSender(t)
		require.Panics(t, func() { NewTeeSender(real, nil) })
	})
}
