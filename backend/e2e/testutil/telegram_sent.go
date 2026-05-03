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
