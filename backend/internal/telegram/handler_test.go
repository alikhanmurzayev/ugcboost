package telegram_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
	tgmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/telegram/mocks"
)

const validAppID = "11111111-2222-3333-4444-555555555555"

func newUpdate(text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			ID:   77,
			Chat: models.Chat{ID: 4242, Type: "private"},
			Text: text,
			From: &models.User{
				ID:        int64(123),
				Username:  "aidana",
				FirstName: "Aidana",
				LastName:  "M.",
			},
		},
	}
}

// matchSend matches *bot.SendMessageParams by chat id and text — the only
// fields the handler sets — without coupling tests to other defaults.
func matchSend(chatID int64, text string) any {
	return mock.MatchedBy(func(p *bot.SendMessageParams) bool {
		id, _ := p.ChatID.(int64)
		return id == chatID && p.Text == text
	})
}

// newRecorderExpectingInbound returns a mock recorder that expects exactly one
// RecordInbound call with the given update. Tests covering private-chat
// dispatch use this helper so the spec invariant (inbound row written before
// dispatcher) stays enforced.
func newRecorderExpectingInbound(t *testing.T, upd *models.Update) *tgmocks.MockMessageRecorder {
	t.Helper()
	rec := tgmocks.NewMockMessageRecorder(t)
	rec.EXPECT().RecordInbound(mock.Anything, upd).Return()
	return rec
}

// newSilentRecorder returns a recorder that should NOT be called — covers
// no-op branches (nil update, non-private chat, from=nil). Any call surfaces
// via mockery's cleanup assertion.
func newSilentRecorder(t *testing.T) *tgmocks.MockMessageRecorder {
	t.Helper()
	return tgmocks.NewMockMessageRecorder(t)
}

func expectFallback(t *testing.T, text string) {
	t.Helper()
	linkSvc := tgmocks.NewMockLinkService(t)
	log := logmocks.NewMockLogger(t)
	spy := tgmocks.NewMockSender(t)
	upd := newUpdate(text)
	rec := newRecorderExpectingInbound(t, upd)
	h := telegram.NewHandler(linkSvc, rec, log)

	spy.EXPECT().SendMessage(mock.Anything, matchSend(4242, telegram.MessageFallback)).
		Return(nil, nil)

	h.Handle(context.Background(), spy, upd)
}

func TestHandler_Handle(t *testing.T) {
	t.Parallel()

	t.Run("nil update is a no-op", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		h.Handle(context.Background(), spy, nil)
	})

	t.Run("update without message is a no-op", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		h.Handle(context.Background(), spy, &models.Update{
			CallbackQuery: &models.CallbackQuery{ID: "cb-1"},
		})
	})

	t.Run("group chat is dropped silently", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		upd := newUpdate("/start " + validAppID)
		upd.Message.Chat.Type = "group"

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("supergroup is dropped silently", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		upd := newUpdate("/start " + validAppID)
		upd.Message.Chat.Type = "supergroup"

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("channel post is dropped silently", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		upd := newUpdate("/start " + validAppID)
		upd.Message.Chat.Type = "channel"

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("From == nil is dropped silently", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		upd := newUpdate("/start " + validAppID)
		upd.Message.From = nil

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("From.ID <= 0 is dropped silently", func(t *testing.T) {
		t.Parallel()
		h := telegram.NewHandler(tgmocks.NewMockLinkService(t), newSilentRecorder(t), logmocks.NewMockLogger(t))
		spy := tgmocks.NewMockSender(t)

		upd := newUpdate("/start " + validAppID)
		upd.Message.From.ID = 0

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("/start without payload → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "/start")
	})

	t.Run("/start with non-UUID payload → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "/start abracadabra")
	})

	t.Run("/start with non-canonical UUID → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "/start urn:uuid:11111111-2222-3333-4444-555555555555")
	})

	t.Run("/start with nil UUID → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "/start 00000000-0000-0000-0000-000000000000")
	})

	t.Run("/start with mixed-case hex UUID → success, no sync reply", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)

		const hexUUID = "aabbccdd-eeff-aabb-ccdd-eeff00112233"
		upd := newUpdate("/start " + hexUUID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.TelegramLinkInput, _ time.Time) {
				require.Equal(t, hexUUID, in.ApplicationID)
			}).
			Return(nil)

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("any other command → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "/help")
	})

	t.Run("plain text → fallback", func(t *testing.T) {
		t.Parallel()
		expectFallback(t, "hi there")
	})

	t.Run("/start <uuid> → success, no sync reply (welcome via async notifier)", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/start " + validAppID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, in domain.TelegramLinkInput, _ time.Time) {
				require.Equal(t, validAppID, in.ApplicationID)
				require.Equal(t, int64(123), in.TelegramUserID)
				require.NotNil(t, in.TelegramUsername)
				require.Equal(t, "aidana", *in.TelegramUsername)
			}).
			Return(nil)
		// No EXPECT on spy.SendMessage — handler must not reply on success.

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("ErrNotFound → application not found reply", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/start " + validAppID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(domain.ErrNotFound)
		spy.EXPECT().SendMessage(mock.Anything, matchSend(4242, telegram.MessageApplicationNotFound)).
			Return(nil, nil)

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("ApplicationAlreadyLinked → already linked reply", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/start " + validAppID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(domain.NewBusinessError(domain.CodeTelegramApplicationAlreadyLinked, ""))
		spy.EXPECT().SendMessage(mock.Anything, matchSend(4242, telegram.MessageApplicationAlreadyLinked)).
			Return(nil, nil)

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("unknown error → internal error reply, logged", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/start " + validAppID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("db down"))
		log.EXPECT().Error(mock.Anything, "telegram link failed", mock.Anything).Once()
		spy.EXPECT().SendMessage(mock.Anything, matchSend(4242, telegram.MessageInternalError)).
			Return(nil, nil)

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("send error is logged and does not panic", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/help")
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		spy.EXPECT().SendMessage(mock.Anything, mock.Anything).Return(nil, errors.New("network down"))
		log.EXPECT().Error(mock.Anything, "telegram send message failed", mock.Anything).Once()

		h.Handle(context.Background(), spy, upd)
	})

	t.Run("panic in LinkService is recovered with stack log", func(t *testing.T) {
		t.Parallel()
		linkSvc := tgmocks.NewMockLinkService(t)
		log := logmocks.NewMockLogger(t)
		spy := tgmocks.NewMockSender(t)
		upd := newUpdate("/start " + validAppID)
		rec := newRecorderExpectingInbound(t, upd)
		h := telegram.NewHandler(linkSvc, rec, log)

		linkSvc.EXPECT().LinkTelegram(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ domain.TelegramLinkInput, _ time.Time) {
				panic("boom")
			}).
			Return(nil) // unreachable but mockery requires a return for typed mocks
		log.EXPECT().Error(mock.Anything, "telegram handler panic", mock.Anything).Once()

		// require.NotPanics asserts the defer recover swallowed the panic.
		require.NotPanics(t, func() {
			h.Handle(context.Background(), spy, upd)
		})
	})
}
