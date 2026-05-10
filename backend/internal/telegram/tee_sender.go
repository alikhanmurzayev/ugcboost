package telegram

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SpyOnlySender records every send into a SentSpyStore and never touches the
// network. Used in local/CI where no TELEGRAM_BOT_TOKEN is configured but
// EnableTestEndpoints is true — the test-API drives the bot in-process.
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
// When a one-shot synthetic failure is registered for the chat_id via
// RegisterFailNext, the call returns the canonical error and records the
// attempt with Err set so the spy log still captures it.
func (s *SpyOnlySender) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	if reason, ok := consumeFailNextFromParams(s.store, params); ok {
		err := newSyntheticTGErr(reason)
		s.store.Record(recordFromParams(params, time.Now().UTC(), err))
		return nil, err
	}
	s.store.Record(recordFromParams(params, time.Now().UTC(), nil))
	return &models.Message{ID: 1}, nil
}

// TeeSender forwards each send to the real upstream Sender and records the
// outcome (including error) in the SentSpyStore. Used on staging where both
// TELEGRAM_BOT_TOKEN and EnableTestEndpoints are set: real Telegram delivery
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
// error handling stays identical to the production-without-spy path. Two
// test-only escape hatches short-circuit the real call before it fires:
// (1) a one-shot fail-next registration returns the synthetic Telegram
// error; (2) a fake-chat registration returns success — needed when the
// synthetic chat_id has no live chat for staging Telegram to reach.
func (t *TeeSender) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	if reason, ok := consumeFailNextFromParams(t.store, params); ok {
		err := newSyntheticTGErr(reason)
		t.store.Record(recordFromParams(params, time.Now().UTC(), err))
		return nil, err
	}
	if isFakeChatFromParams(t.store, params) {
		t.store.Record(recordFromParams(params, time.Now().UTC(), nil))
		return &models.Message{ID: 1}, nil
	}
	msg, err := t.real.SendMessage(ctx, params)
	t.store.Record(recordFromParams(params, time.Now().UTC(), err))
	return msg, err
}

// consumeFailNextFromParams resolves the chat_id from SendMessageParams
// and asks the store whether a synthetic failure is queued. Lives here
// (next to the senders) rather than on SentSpyStore because it depends on
// the bot-package shape of params.
func consumeFailNextFromParams(store *SentSpyStore, params *bot.SendMessageParams) (string, bool) {
	if params == nil {
		return "", false
	}
	chatID, ok := params.ChatID.(int64)
	if !ok {
		return "", false
	}
	return store.consumeFailNext(chatID)
}

// isFakeChatFromParams resolves the chat_id and asks the store whether the
// chat is registered as test-synthetic. Mirrors consumeFailNextFromParams in
// shape so the SendMessage flow reads top-down.
func isFakeChatFromParams(store *SentSpyStore, params *bot.SendMessageParams) bool {
	if params == nil {
		return false
	}
	chatID, ok := params.ChatID.(int64)
	if !ok {
		return false
	}
	return store.IsFakeChat(chatID)
}
