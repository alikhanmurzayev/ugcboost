package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// telegramSentPollInterval is short enough to keep test wall time low while
// giving the post-commit notify goroutine room to flush in slow CI runs.
const telegramSentPollInterval = 100 * time.Millisecond

// RegisterTelegramSpyFailNext queues a one-shot synthetic Telegram failure
// for the chat_id via POST /test/telegram/spy/fail-next. The next outbound
// SendMessage to that chat_id returns the canonical
// "Forbidden: bot was blocked by the user" error so chunk-12 partial-success
// e2e exercises the bot_blocked branch without needing a real blocked user.
// Pass reason="" to use the default. The registration is consumed by the
// next send (FIFO per chat_id).
func RegisterTelegramSpyFailNext(t *testing.T, chatID int64, reason string) {
	t.Helper()
	client := NewTestClient(t)
	body := testclient.TelegramSpyFailNextJSONRequestBody{ChatId: chatID}
	if reason != "" {
		r := reason
		body.Reason = &r
	}
	resp, err := client.TelegramSpyFailNextWithResponse(context.Background(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode())
}

// RegisterTelegramSpyFakeChat marks chatID as test-synthetic so TeeSender
// skips the real bot's SendMessage call. Required for chunk-12 e2e where
// the synthetic creator chat_id has no live counterpart in staging
// Telegram and the real bot would otherwise return "chat not found" for
// every send. SpyOnlySender ignores the registration (it never hits a
// real bot anyway). Idempotent; safe to call once per creator at setup.
func RegisterTelegramSpyFakeChat(t *testing.T, chatID int64) {
	t.Helper()
	client := NewTestClient(t)
	resp, err := client.TelegramSpyFakeChatWithResponse(context.Background(),
		testclient.TelegramSpyFakeChatJSONRequestBody{ChatId: chatID})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode())
}

// TelegramSentOptions controls the polling behaviour of WaitForTelegramSent.
// Since narrows the spy lookup; ExpectCount is the size the helper waits
// for. Timeout caps total wall time spent polling.
type TelegramSentOptions struct {
	Since       time.Time
	ExpectCount int
	Timeout     time.Duration
}

// WaitForTelegramSent polls /test/telegram/sent until ExpectCount records
// for chatID at-or-after Since are visible, then returns them. Fails the
// test on Timeout so flaky behaviour surfaces immediately. The post-commit
// notify path runs asynchronously in the service, so any e2e assert on
// outbound TG content must go through this helper, not a one-shot read.
func WaitForTelegramSent(t *testing.T, chatID int64, opts TelegramSentOptions) []testclient.TelegramSentMessage {
	t.Helper()
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	deadline := time.Now().Add(opts.Timeout)

	client := NewTestClient(t)
	params := &testclient.GetTelegramSentParams{ChatId: chatID}
	if !opts.Since.IsZero() {
		s := opts.Since
		params.Since = &s
	}

	var lastSeen []testclient.TelegramSentMessage
	for {
		resp, err := client.GetTelegramSentWithResponse(context.Background(), params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		lastSeen = resp.JSON200.Data.Messages
		if len(lastSeen) >= opts.ExpectCount {
			return lastSeen
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d sent messages for chat %d, got %d after %s", opts.ExpectCount, chatID, len(lastSeen), opts.Timeout)
		}
		time.Sleep(telegramSentPollInterval)
	}
}

// EnsureNoNewTelegramSent polls /test/telegram/sent for `window` time and
// fails the test as soon as any record at-or-after `since` appears for
// chatID. Use it for negative assertions ("notify must NOT fire") instead
// of `time.Sleep` + one-shot read — sleep is flaky on slow CI, polling
// fails fast on positive evidence and only succeeds after observing
// stability over the full window.
func EnsureNoNewTelegramSent(t *testing.T, chatID int64, since time.Time, window time.Duration) {
	t.Helper()
	deadline := time.Now().Add(window)
	client := NewTestClient(t)
	s := since
	params := &testclient.GetTelegramSentParams{ChatId: chatID, Since: &s}
	for time.Now().Before(deadline) {
		resp, err := client.GetTelegramSentWithResponse(context.Background(), params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		if len(resp.JSON200.Data.Messages) > 0 {
			t.Fatalf("expected zero sent messages for chat %d in %s window, got %d (first: %q)",
				chatID, window, len(resp.JSON200.Data.Messages), resp.JSON200.Data.Messages[0].Text)
		}
		time.Sleep(telegramSentPollInterval)
	}
}
