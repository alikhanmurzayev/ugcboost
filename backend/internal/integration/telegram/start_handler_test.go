package telegram_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

const validAppUUID = "11111111-2222-3333-4444-555555555555"

type startHandlerFixture struct {
	handler  *telegram.StartHandler
	client   *mocks.MockClient
	service  *mocks.MockLinkService
	messages *mocks.MockMessages
	logger   *logmocks.MockLogger
}

func newStartHandlerFixture(t *testing.T) startHandlerFixture {
	t.Helper()
	client := mocks.NewMockClient(t)
	service := mocks.NewMockLinkService(t)
	messages := mocks.NewMockMessages(t)
	log := logmocks.NewMockLogger(t)
	return startHandlerFixture{
		handler:  telegram.NewStartHandler(service, client, messages, log),
		client:   client,
		service:  service,
		messages: messages,
		logger:   log,
	}
}

func TestStartHandler_Handle(t *testing.T) {
	t.Parallel()

	t.Run("invalid UUID payload replies InvalidPayload without calling service", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.messages.EXPECT().InvalidPayload().Return("invalid-text")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "invalid-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, "not-a-uuid")
	})

	t.Run("happy path: link service success → LinkSuccess reply", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		update := telegram.IncomingUpdate{
			ChatID:    100,
			UserID:    7000,
			Username:  pointer.ToString("test_42"),
			FirstName: pointer.ToString("Айдана"),
			LastName:  pointer.ToString("Муратова"),
		}

		var capturedInput domain.TelegramLinkInput
		fix.service.EXPECT().LinkTelegram(mock.Anything,
			mock.AnythingOfType("domain.TelegramLinkInput"),
			mock.AnythingOfType("time.Time")).
			Run(func(_ context.Context, in domain.TelegramLinkInput, _ time.Time) {
				capturedInput = in
			}).
			Return(&domain.TelegramLinkResult{ApplicationID: validAppUUID, TelegramUserID: 7000}, nil)

		fix.messages.EXPECT().LinkSuccess().Return("link-success")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "link-success").Return(nil)

		fix.handler.Handle(context.Background(), update, validAppUUID)
		require.Equal(t, validAppUUID, capturedInput.ApplicationID)
		require.Equal(t, int64(7000), capturedInput.TgUserID)
		require.Equal(t, pointer.ToString("test_42"), capturedInput.TgUsername)
		require.Equal(t, pointer.ToString("Айдана"), capturedInput.TgFirstName)
		require.Equal(t, pointer.ToString("Муратова"), capturedInput.TgLastName)
	})

	t.Run("application not found → ApplicationNotFound reply", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, domain.ErrNotFound)
		fix.messages.EXPECT().ApplicationNotFound().Return("nf-text")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "nf-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})

	t.Run("application not active → ApplicationNotActive reply", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, domain.NewBusinessError(domain.CodeTelegramApplicationNotActive, "msg"))
		fix.messages.EXPECT().ApplicationNotActive().Return("inactive-text")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "inactive-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})

	t.Run("application already linked to other TG → ApplicationAlreadyLinked reply", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, "msg"))
		fix.messages.EXPECT().ApplicationAlreadyLinked().Return("app-linked-text")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "app-linked-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})

	t.Run("account already linked to other application → AccountAlreadyLinked reply", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, domain.NewBusinessError(domain.CodeTelegramAccountAlreadyLinked, "msg"))
		fix.messages.EXPECT().AccountAlreadyLinked().Return("acct-linked-text")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "acct-linked-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})

	t.Run("unexpected error → InternalError reply, error-logged", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("boom"))
		fix.messages.EXPECT().InternalError().Return("internal-text")
		fix.logger.EXPECT().Error(mock.Anything, "telegram start handler internal error",
			mock.Anything).Once()
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "internal-text").Return(nil)

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})

	t.Run("send failure is logged but does not panic", func(t *testing.T) {
		t.Parallel()
		fix := newStartHandlerFixture(t)
		fix.service.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.TelegramLinkResult{}, nil)
		fix.messages.EXPECT().LinkSuccess().Return("link-success")
		fix.client.EXPECT().SendMessage(mock.Anything, int64(100), "link-success").
			Return(errors.New("net down"))
		fix.logger.EXPECT().Warn(mock.Anything, "telegram start handler reply failed",
			mock.Anything).Once()

		fix.handler.Handle(context.Background(), telegram.IncomingUpdate{ChatID: 100, UserID: 7000}, validAppUUID)
	})
}
