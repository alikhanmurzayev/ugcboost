package telegram

import (
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"
)

func TestSentSpyStore_RecordAndList(t *testing.T) {
	t.Parallel()

	t.Run("returns inserted records in order", func(t *testing.T) {
		t.Parallel()
		s := NewSentSpyStore()
		now := time.Now().UTC()
		s.Record(SentRecord{ChatID: 1, Text: "first", SentAt: now})
		s.Record(SentRecord{ChatID: 1, Text: "second", SentAt: now.Add(time.Second)})

		got := s.List(SentFilter{})
		require.Len(t, got, 2)
		require.Equal(t, "first", got[0].Text)
		require.Equal(t, "second", got[1].Text)
	})

	t.Run("filter by chat id", func(t *testing.T) {
		t.Parallel()
		s := NewSentSpyStore()
		s.Record(SentRecord{ChatID: 1, Text: "a", SentAt: time.Now().UTC()})
		s.Record(SentRecord{ChatID: 2, Text: "b", SentAt: time.Now().UTC()})

		got := s.List(SentFilter{ChatID: 2})
		require.Len(t, got, 1)
		require.Equal(t, "b", got[0].Text)
	})

	t.Run("filter by since drops earlier records", func(t *testing.T) {
		t.Parallel()
		s := NewSentSpyStore()
		now := time.Now().UTC()
		s.Record(SentRecord{ChatID: 1, Text: "old", SentAt: now.Add(-time.Hour)})
		s.Record(SentRecord{ChatID: 1, Text: "new", SentAt: now})

		got := s.List(SentFilter{Since: now.Add(-time.Minute)})
		require.Len(t, got, 1)
		require.Equal(t, "new", got[0].Text)
	})

	t.Run("ring buffer evicts oldest when capacity hit", func(t *testing.T) {
		t.Parallel()
		s := NewSentSpyStore()
		for i := 0; i < sentSpyStoreCapacity+5; i++ {
			s.Record(SentRecord{ChatID: 1, Text: "x", SentAt: time.Now().UTC()})
		}
		got := s.List(SentFilter{})
		require.Len(t, got, sentSpyStoreCapacity)
	})
}

func TestRecordFromParams(t *testing.T) {
	t.Parallel()

	t.Run("populates all fields from typed chat id", func(t *testing.T) {
		t.Parallel()
		now := time.Now().UTC()
		params := &bot.SendMessageParams{
			ChatID: int64(42),
			Text:   "hi",
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "btn"}}},
			},
		}
		rec := recordFromParams(params, now, nil)
		require.Equal(t, int64(42), rec.ChatID)
		require.Equal(t, "hi", rec.Text)
		require.NotNil(t, rec.ReplyMarkup)
		require.Equal(t, now, rec.SentAt)
		require.Empty(t, rec.Err)
	})

	t.Run("captures error string", func(t *testing.T) {
		t.Parallel()
		rec := recordFromParams(&bot.SendMessageParams{ChatID: int64(1)}, time.Now(), errSentinel{msg: "boom"})
		require.Equal(t, "boom", rec.Err)
	})

	t.Run("non-int64 chat id leaves ChatID zero", func(t *testing.T) {
		t.Parallel()
		rec := recordFromParams(&bot.SendMessageParams{ChatID: "string-not-supported"}, time.Now(), nil)
		require.Zero(t, rec.ChatID)
	})

	t.Run("nil params returns blank record", func(t *testing.T) {
		t.Parallel()
		rec := recordFromParams(nil, time.Now(), nil)
		require.Zero(t, rec.ChatID)
		require.Empty(t, rec.Text)
	})
}

type errSentinel struct{ msg string }

func (e errSentinel) Error() string { return e.msg }
