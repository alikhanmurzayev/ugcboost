package telegram_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestDispatcher_Dispatch(t *testing.T) {
	t.Parallel()

	t.Run("/start with payload routes to start handler", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		update := telegram.IncomingUpdate{ChatID: 100, UserID: 7000, Text: "/start abc-uuid"}
		start.EXPECT().Handle(mock.Anything, update, "abc-uuid").Return()

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), update)
	})

	t.Run("/start without payload replies with StartNoPayload", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		messages.EXPECT().StartNoPayload().Return("nopayload-text")
		client.EXPECT().SendMessage(mock.Anything, int64(100), "nopayload-text").Return(nil)

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), telegram.IncomingUpdate{ChatID: 100, Text: "/start"})
	})

	t.Run("/start with whitespace-only payload is treated as no payload", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		messages.EXPECT().StartNoPayload().Return("nopayload-text")
		client.EXPECT().SendMessage(mock.Anything, int64(100), "nopayload-text").Return(nil)

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), telegram.IncomingUpdate{ChatID: 100, Text: "  /start  "})
	})

	t.Run("unknown command falls through to Fallback", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		messages.EXPECT().Fallback().Return("fallback-text")
		client.EXPECT().SendMessage(mock.Anything, int64(100), "fallback-text").Return(nil)

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), telegram.IncomingUpdate{ChatID: 100, Text: "/help"})
	})

	t.Run("plain text falls through to Fallback", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		messages.EXPECT().Fallback().Return("fallback-text")
		client.EXPECT().SendMessage(mock.Anything, int64(100), "fallback-text").Return(nil)

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), telegram.IncomingUpdate{ChatID: 100, Text: "Hi there"})
	})

	t.Run("send failure is logged but does not propagate", func(t *testing.T) {
		t.Parallel()
		client := mocks.NewMockClient(t)
		start := mocks.NewMockStartCommandHandler(t)
		messages := mocks.NewMockMessages(t)
		log := logmocks.NewMockLogger(t)

		messages.EXPECT().Fallback().Return("fallback-text")
		client.EXPECT().SendMessage(mock.Anything, int64(100), "fallback-text").
			Return(errors.New("network down"))
		log.EXPECT().Warn(mock.Anything, "telegram dispatcher reply failed",
			mock.Anything).Once()

		d := telegram.NewDispatcher(client, start, messages, log)
		d.Dispatch(context.Background(), telegram.IncomingUpdate{ChatID: 100, Text: "Hi"})
	})
}
