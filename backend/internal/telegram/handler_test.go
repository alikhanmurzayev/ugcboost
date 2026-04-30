package telegram_test

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// spySender is a minimal Sender implementation that records every
// SendMessage call so assertions can verify what the handler actually sent.
type spySender struct {
	sent []*bot.SendMessageParams
}

func (s *spySender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	s.sent = append(s.sent, params)
	return &models.Message{ID: len(s.sent)}, nil
}

func TestHandler_Handle(t *testing.T) {
	t.Parallel()

	t.Run("text message → hello world reply", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler()
		spy := &spySender{}
		update := &models.Update{
			Message: &models.Message{
				Chat: models.Chat{ID: 42},
				Text: "hi",
			},
		}

		h.Handle(context.Background(), spy, update)

		require.Len(t, spy.sent, 1)
		require.Equal(t, int64(42), spy.sent[0].ChatID)
		require.Equal(t, telegram.HelloWorldReply, spy.sent[0].Text)
	})

	t.Run("nil update is a no-op", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler()
		spy := &spySender{}

		h.Handle(context.Background(), spy, nil)

		require.Empty(t, spy.sent)
	})

	t.Run("update without message is a no-op", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler()
		spy := &spySender{}
		update := &models.Update{
			CallbackQuery: &models.CallbackQuery{ID: "cb-1"},
		}

		h.Handle(context.Background(), spy, update)

		require.Empty(t, spy.sent)
	})
}
