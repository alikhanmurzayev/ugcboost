package telegram

import (
	"errors"
	"sync"
	"time"

	"github.com/go-telegram/bot"
)

// sentSpyStoreCapacity caps the in-memory ring of recorded outbound messages.
// Sized for staging E2E concurrency: ~50 parallel tests × ~10 messages each
// with healthy headroom — older records are evicted FIFO.
const sentSpyStoreCapacity = 5000

// SentRecord captures one outbound Telegram send for test inspection.
// ReplyMarkup is kept as the raw value the sender received so e2e tests can
// assert on InlineKeyboardMarkup → WebApp.URL without re-parsing.
type SentRecord struct {
	ChatID      int64
	Text        string
	ReplyMarkup any
	SentAt      time.Time
	Err         string
}

// SentFilter narrows a SentSpyStore.List query. Zero-value (Since.IsZero,
// ChatID == 0) returns the full ring.
type SentFilter struct {
	ChatID int64
	Since  time.Time
}

// SentSpyStore is the thread-safe ring of recorded outbound messages.
// Created only when EnableTestEndpoints is true. The same store also owns
// per-chat one-shot synthetic-failure registrations used by chunk-12 e2e
// to exercise partial-success delivery without a real blocked-by-user, and
// a "fake chat" set for tests that need TeeSender to skip the upstream
// real-bot call entirely (synthetic chat_ids cannot be reached by a live
// bot — staging would always reply "chat not found", breaking the chunk-12
// happy-path contract on `undelivered=[]`).
type SentSpyStore struct {
	mu        sync.Mutex
	records   []SentRecord
	failNext  map[int64]string // chatID → reason text for the next send
	fakeChats map[int64]struct{}
}

// NewSentSpyStore returns a store with the package-level capacity. The
// capacity is fixed so test infrastructure cannot accidentally tune it
// down to a value that lets parallel e2e runs evict each other's records.
func NewSentSpyStore() *SentSpyStore {
	return &SentSpyStore{
		records:   make([]SentRecord, 0, sentSpyStoreCapacity),
		failNext:  make(map[int64]string),
		fakeChats: make(map[int64]struct{}),
	}
}

// RegisterFakeChat marks chatID as test-synthetic so TeeSender skips the
// real-bot SendMessage call and returns success directly. SpyOnlySender
// ignores this since it is already recording-only. Used by chunk-12 e2e
// happy-path scenarios where the synthetic Telegram user id has no live
// chat for the bot to reach.
func (s *SentSpyStore) RegisterFakeChat(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fakeChats[chatID] = struct{}{}
}

// IsFakeChat reports whether RegisterFakeChat was called for chatID.
func (s *SentSpyStore) IsFakeChat(chatID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.fakeChats[chatID]
	return ok
}

// RegisterFailNext queues a one-shot synthetic failure for the next
// SendMessage call to chatID. reason is the verbatim error string the
// sender will return; pass `""` to use the canonical "Forbidden: bot was
// blocked by the user" payload that maps to NotifyFailureReasonBotBlocked.
func (s *SentSpyStore) RegisterFailNext(chatID int64, reason string) {
	if reason == "" {
		reason = "Forbidden: bot was blocked by the user"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext[chatID] = reason
}

// consumeFailNext returns the queued reason string for chatID and clears
// the registration so the failure fires exactly once. Returns ("", false)
// when no failure is queued.
func (s *SentSpyStore) consumeFailNext(chatID int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reason, ok := s.failNext[chatID]
	if !ok {
		return "", false
	}
	delete(s.failNext, chatID)
	return reason, true
}

// newSyntheticTGErr returns an error whose .Error() is exactly reason so
// MapTelegramErrorToReason classifies it like the real SDK counterpart
// ("Forbidden: ..." → bot_blocked, anything else → unknown).
func newSyntheticTGErr(reason string) error {
	return errors.New(reason)
}

// Record appends one send result. When the ring is full the oldest record
// is dropped (FIFO eviction).
func (s *SentSpyStore) Record(rec SentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) >= sentSpyStoreCapacity {
		s.records = s.records[1:]
	}
	s.records = append(s.records, rec)
}

// List returns a copy of records matching the filter, in insertion order.
// The returned slice is owned by the caller and safe to mutate.
func (s *SentSpyStore) List(filter SentFilter) []SentRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SentRecord, 0, len(s.records))
	for _, r := range s.records {
		if filter.ChatID != 0 && r.ChatID != filter.ChatID {
			continue
		}
		if !filter.Since.IsZero() && r.SentAt.Before(filter.Since) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// recordFromParams normalises bot.SendMessageParams into a SentRecord.
// chatID extraction mirrors the bot library's own marshalling rules — only
// the int64 case is supported by Telegram for private DMs and is the only
// one our flows produce.
func recordFromParams(params *bot.SendMessageParams, sentAt time.Time, sendErr error) SentRecord {
	rec := SentRecord{SentAt: sentAt}
	if params != nil {
		if cid, ok := params.ChatID.(int64); ok {
			rec.ChatID = cid
		}
		rec.Text = params.Text
		rec.ReplyMarkup = params.ReplyMarkup
	}
	if sendErr != nil {
		rec.Err = sendErr.Error()
	}
	return rec
}
