package telegram_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

type spySender struct {
	sent    []*bot.SendMessageParams
	sendErr error
}

func (s *spySender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	s.sent = append(s.sent, params)
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &models.Message{ID: len(s.sent)}, nil
}

func TestHandler_Handle(t *testing.T) {
	t.Parallel()

	t.Run("text message → hello world reply", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		h := telegram.NewHandler(log)
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
		require.Equal(t, "Hello, world!", spy.sent[0].Text)
	})

	t.Run("nil update is a no-op", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		h := telegram.NewHandler(log)
		spy := &spySender{}

		h.Handle(context.Background(), spy, nil)

		require.Empty(t, spy.sent)
	})

	t.Run("update without message is a no-op", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		h := telegram.NewHandler(log)
		spy := &spySender{}
		update := &models.Update{
			CallbackQuery: &models.CallbackQuery{ID: "cb-1"},
		}

		h.Handle(context.Background(), spy, update)

		require.Empty(t, spy.sent)
	})

	t.Run("send error is logged", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "telegram send message failed", mock.Anything).Once()
		h := telegram.NewHandler(log)
		spy := &spySender{sendErr: errors.New("network down")}
		update := &models.Update{
			Message: &models.Message{Chat: models.Chat{ID: 7}, Text: "hi"},
		}

		h.Handle(context.Background(), spy, update)

		require.Len(t, spy.sent, 1)
	})
}
