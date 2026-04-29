package telegram

import (
	"context"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// Test-only re-exports of internals so external _test files can drive the
// package without exposing surface area to production callers.

// SpyClient is the test-visible facade over the in-memory spy implementation.
type SpyClient struct{ inner *spyClient }

// NewSpyClientForTest constructs a fresh spy. Identical to the implementation
// NewClient picks when EnableTestEndpoints is set.
func NewSpyClientForTest(log logger.Logger) *SpyClient {
	return &SpyClient{inner: newSpyClient(log)}
}

// SendMessage records an outgoing message via the production spy code path
// (mutex-guarded).
func (s *SpyClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	return s.inner.SendMessage(ctx, chatID, text)
}

// Drain returns and clears the buffered messages for chatID.
func (s *SpyClient) Drain(chatID int64) []SentMessage {
	return s.inner.Drain(chatID)
}

// SetRetryDelayForTest overrides the runner's retry sleep so tests do not
// wait the production 10s window.
func (r *PollingRunner) SetRetryDelayForTest(d time.Duration) {
	r.retryDelay = d
}

// SetRealClientAPIBaseForTest overrides the Bot API host of a realClient with
// an httptest server URL. Returns silently for non-real clients.
func SetRealClientAPIBaseForTest(c Client, base string) {
	if rc, ok := c.(*realClient); ok {
		rc.apiBase = base
	}
}
