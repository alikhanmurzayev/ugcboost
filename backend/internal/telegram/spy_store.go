package telegram

import (
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
// Created only when EnableTestEndpoints is true.
type SentSpyStore struct {
	mu      sync.Mutex
	records []SentRecord
}

// NewSentSpyStore returns a store with the package-level capacity. The
// capacity is fixed so test infrastructure cannot accidentally tune it
// down to a value that lets parallel e2e runs evict each other's records.
func NewSentSpyStore() *SentSpyStore {
	return &SentSpyStore{records: make([]SentRecord, 0, sentSpyStoreCapacity)}
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
