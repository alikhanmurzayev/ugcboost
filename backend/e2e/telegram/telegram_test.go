// Package telegram — E2E тесты Telegram-бота.
//
// Тесты используют POST /test/telegram/message: ручка собирает synthetic
// Update, прогоняет через тот же in-process handler, что и production
// long polling, и возвращает ответы, перехваченные in-memory spy Sender.
package telegram_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestSendTelegramMessage_HelloWorld(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	resp, err := tc.SendTelegramMessageWithResponse(context.Background(), testclient.SendTelegramMessageJSONRequestBody{
		ChatId: 4242,
		Text:   "/start",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	require.Len(t, resp.JSON200.Replies, 1)
	require.Equal(t, int64(4242), resp.JSON200.Replies[0].ChatId)
	require.Equal(t, "Hello, world!", resp.JSON200.Replies[0].Text)
}

func TestSendTelegramMessage_ValidationErrors(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)

	resp, err := tc.SendTelegramMessageWithResponse(context.Background(), testclient.SendTelegramMessageJSONRequestBody{
		ChatId: 1,
		Text:   "",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}
