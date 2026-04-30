// Package telegram — E2E тесты на skeleton Telegram-бота.
//
// Бэкенд работает в test-режиме: production long-polling не стартует
// (TELEGRAM_BOT_TOKEN либо пустой, либо EnableTestEndpoints=true гасит
// runner). Обновления инжектятся синхронно через POST /test/telegram/message —
// тестовая ручка собирает Update, зовёт in-process telegram.Handler с
// in-memory spy Sender и возвращает зафиксированные ответы. Это полный
// round-trip через настоящий бизнес-слой, но без живой Telegram-инфры.
//
// TestSendTelegramMessage_HelloWorld — бот должен вернуть "Hello, world!"
// на любой текстовый ввод. Этот тест — sanity-чек скелета: подтверждает,
// что транспорт↔бизнес-разделение работает (handler принимает Update,
// отвечает через Sender, ответ ловится spy'ем и возвращается клиенту).
//
// TestSendTelegramMessage_ValidationErrors — пустой text отвергается
// сервером с 422 без захода в handler.
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

	// Empty text → 422 before the handler runs (server-side guard).
	resp, err := tc.SendTelegramMessageWithResponse(context.Background(), testclient.SendTelegramMessageJSONRequestBody{
		ChatId: 1,
		Text:   "",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}
