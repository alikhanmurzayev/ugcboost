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

func TestRecordingSender_SendMessage(t *testing.T) {
	t.Parallel()

	t.Run("inner returns msg+nil: recorder gets identical args, caller gets msg+nil", func(t *testing.T) {
		t.Parallel()
		inner := tgmocks.NewMockSender(t)
		recorder := tgmocks.NewMockMessageRecorder(t)

		params := &bot.SendMessageParams{ChatID: int64(42), Text: "hi"}
		expectedMsg := &models.Message{ID: 99}
		inner.EXPECT().SendMessage(mock.Anything, params).Return(expectedMsg, nil)
		recorder.EXPECT().RecordOutbound(mock.Anything, params, expectedMsg, nil).Return()

		sender := NewRecordingSender(inner, recorder)
		msg, err := sender.SendMessage(context.Background(), params)
		require.NoError(t, err)
		require.Equal(t, expectedMsg, msg)
	})

	t.Run("inner returns nil+err: recorder gets identical err, caller gets nil+err verbatim", func(t *testing.T) {
		t.Parallel()
		inner := tgmocks.NewMockSender(t)
		recorder := tgmocks.NewMockMessageRecorder(t)

		params := &bot.SendMessageParams{ChatID: int64(42), Text: "hi"}
		sendErr := errors.New("Forbidden: bot was blocked by the user")
		inner.EXPECT().SendMessage(mock.Anything, params).Return(nil, sendErr)
		recorder.EXPECT().RecordOutbound(mock.Anything, params, (*models.Message)(nil), sendErr).Return()

		sender := NewRecordingSender(inner, recorder)
		msg, err := sender.SendMessage(context.Background(), params)
		require.Nil(t, msg)
		require.ErrorIs(t, err, sendErr)
	})
}
