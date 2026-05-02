package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	tgmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/telegram/mocks"
)

func TestSendVerificationNotification(t *testing.T) {
	t.Parallel()

	const tmaURL = "https://tma.ugcboost.test"

	t.Run("posts message with WebApp inline keyboard", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		var captured *bot.SendMessageParams
		sender.EXPECT().SendMessage(mock.Anything, mock.AnythingOfType("*bot.SendMessageParams")).
			Run(func(_ context.Context, params *bot.SendMessageParams) {
				captured = params
			}).
			Return(&models.Message{ID: 1}, nil)

		err := SendVerificationNotification(context.Background(), sender, 12345, tmaURL)
		require.NoError(t, err)
		require.NotNil(t, captured)
		require.Equal(t, int64(12345), captured.ChatID)
		require.Equal(t, MessageVerificationApproved, captured.Text)

		markup, ok := captured.ReplyMarkup.(models.InlineKeyboardMarkup)
		require.True(t, ok, "reply markup must be inline keyboard")
		require.Len(t, markup.InlineKeyboard, 1)
		require.Len(t, markup.InlineKeyboard[0], 1)
		btn := markup.InlineKeyboard[0][0]
		require.Equal(t, MessageVerificationApprovedButton, btn.Text)
		require.NotNil(t, btn.WebApp)
		require.Equal(t, tmaURL, btn.WebApp.URL)
	})

	t.Run("propagates send error", func(t *testing.T) {
		t.Parallel()
		sender := tgmocks.NewMockSender(t)
		sender.EXPECT().SendMessage(mock.Anything, mock.AnythingOfType("*bot.SendMessageParams")).
			Return(nil, errors.New("upstream 5xx"))

		err := SendVerificationNotification(context.Background(), sender, 999, tmaURL)
		require.ErrorContains(t, err, "upstream 5xx")
	})
}
