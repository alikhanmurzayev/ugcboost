package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("EnableTestEndpoints picks the spy client", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{EnableTestEndpoints: true}
		client, spy, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NotNil(t, spy)
	})

	t.Run("TelegramMock picks noop client (no spy)", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Debug(mock.Anything, "telegram noop SendMessage", mock.Anything).Maybe()
		cfg := &config.Config{TelegramMock: true}
		client, spy, err := telegram.NewClient(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.Nil(t, spy)
	})

	t.Run("Empty token picks noop client (no spy)", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{}
		client, spy, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
		require.NoError(t, err)
		require.NotNil(t, client)
		require.Nil(t, spy)
	})

	t.Run("With token picks real client (no spy)", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{TelegramBotToken: "test-token"}
		client, spy, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
		require.NoError(t, err)
		require.NotNil(t, client)
		require.Nil(t, spy)
	})
}

func TestNoopClient(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{TelegramMock: true}
	log := logmocks.NewMockLogger(t)
	// noopClient.SendMessage emits a single Debug — we accept any args.
	log.EXPECT().Debug(mock.Anything, "telegram noop SendMessage", mock.Anything).Maybe()

	client, _, err := telegram.NewClient(cfg, log)
	require.NoError(t, err)
	require.NoError(t, client.SendMessage(context.Background(), 1, "hi"))
	got, err := client.GetUpdates(context.Background(), 0, time.Second)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestRealClient_SendMessageAndGetUpdates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, float64(123), body["chat_id"])
			require.Equal(t, "hello", body["text"])
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[
				{"update_id":42,"message":{"chat":{"id":555},"from":{"id":777,"username":"u","first_name":"F","last_name":"L"},"text":"/start abc"}},
				{"update_id":43,"message":{"chat":{"id":556}}}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{TelegramBotToken: "test-token"}
	log := logmocks.NewMockLogger(t)
	client, _, err := telegram.NewClient(cfg, log)
	require.NoError(t, err)
	telegram.SetRealClientAPIBaseForTest(client, srv.URL)

	// SendMessage hits /sendMessage and the test server asserts the body.
	require.NoError(t, client.SendMessage(context.Background(), 123, "hello"))

	// GetUpdates returns the message with full metadata; updates without
	// from/message are filtered out.
	updates, err := client.GetUpdates(context.Background(), 0, time.Second)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, int64(42), updates[0].UpdateID)
	require.Equal(t, int64(555), updates[0].ChatID)
	require.Equal(t, int64(777), updates[0].UserID)
	require.Equal(t, "/start abc", updates[0].Text)
	require.NotNil(t, updates[0].Username)
	require.Equal(t, "u", *updates[0].Username)
	require.NotNil(t, updates[0].FirstName)
	require.Equal(t, "F", *updates[0].FirstName)
	require.NotNil(t, updates[0].LastName)
	require.Equal(t, "L", *updates[0].LastName)
}

func TestRealClient_APIErrorPropagates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error_code":409,"description":"terminated by other getUpdates request"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{TelegramBotToken: "test-token"}
	client, _, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
	require.NoError(t, err)
	telegram.SetRealClientAPIBaseForTest(client, srv.URL)

	_, err = client.GetUpdates(context.Background(), 0, time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "409")
	require.Contains(t, err.Error(), "terminated by other")
}

func TestRealClient_DecodeErrorWraps(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{TelegramBotToken: "test-token"}
	client, _, err := telegram.NewClient(cfg, logmocks.NewMockLogger(t))
	require.NoError(t, err)
	telegram.SetRealClientAPIBaseForTest(client, srv.URL)

	err = client.SendMessage(context.Background(), 1, "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode")
}
