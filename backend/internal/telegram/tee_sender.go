package telegram

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SpyOnlySender records every send into a SentSpyStore and never touches the
// network. Used in local/CI when TELEGRAM_MOCK=true.
type SpyOnlySender struct {
	store *SentSpyStore
}

// NewSpyOnlySender returns a sender that only writes into the spy store.
// store must be non-nil — there is no point constructing a spy-only sender
// with no place to write to, and a nil store would mask configuration bugs.
func NewSpyOnlySender(store *SentSpyStore) *SpyOnlySender {
	if store == nil {
		panic("telegram: NewSpyOnlySender requires non-nil store")
	}
	return &SpyOnlySender{store: store}
}

// SendMessage records the params and returns a synthetic Message{ID: 1} so
// callers that read the ID for follow-up flows do not get a nil panic.
func (s *SpyOnlySender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	s.store.Record(recordFromParams(params, time.Now().UTC(), nil))
	return &models.Message{ID: 1}, nil
}

// TeeSender forwards each send to the real upstream Sender and records the
// outcome (including error) in the SentSpyStore. Used in staging where
// EnableTestEndpoints=true and TELEGRAM_MOCK=false: real Telegram delivery
// happens AND e2e tests can inspect what the backend sent.
type TeeSender struct {
	real  Sender
	store *SentSpyStore
}

// NewTeeSender wraps real with spy-recording. Both arguments must be non-nil.
func NewTeeSender(real Sender, store *SentSpyStore) *TeeSender {
	if real == nil {
		panic("telegram: NewTeeSender requires non-nil real sender")
	}
	if store == nil {
		panic("telegram: NewTeeSender requires non-nil store")
	}
	return &TeeSender{real: real, store: store}
}

// SendMessage delegates to real and records the attempt regardless of error.
// The error from real is returned to the caller unchanged so service-level
// error handling stays identical to the production-without-spy path.
func (t *TeeSender) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	msg, err := t.real.SendMessage(ctx, params)
	t.store.Record(recordFromParams(params, time.Now().UTC(), err))
	return msg, err
}
