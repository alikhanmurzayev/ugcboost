package domain

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Direction enum mirrored from the OpenAPI contract (TelegramMessageDirection).
// Stored verbatim in telegram_messages.direction (CHECK constraint).
const (
	TelegramMessageDirectionInbound  = "inbound"
	TelegramMessageDirectionOutbound = "outbound"
)

// Status enum for outbound rows. NULL for inbound; inbound rows have no
// delivery outcome.
const (
	TelegramMessageStatusSent   = "sent"
	TelegramMessageStatusFailed = "failed"
)

// ErrTelegramMessageAlreadyRecorded surfaces when the partial UNIQUE index on
// (chat_id, telegram_message_id) WHERE direction='inbound' rejects a retried
// Telegram update. Repo translates pgconn 23505 into this sentinel so the
// recorder logs the duplicate at Debug, not Error.
var ErrTelegramMessageAlreadyRecorded = errors.New("telegram message already recorded")

// TelegramMessagesCursor is the keyset cursor for /telegram-messages pagination.
// Ordering is DESC by (created_at, id); the cursor names the last row of the
// previous page so the next page returns rows strictly older than it.
type TelegramMessagesCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

// EncodeTelegramMessagesCursor packs the cursor into a URL-safe base64 string.
// Wire format is base64(JSON) — the schema is private to the server; clients
// only see an opaque token.
func EncodeTelegramMessagesCursor(c TelegramMessagesCursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode telegram_messages cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeTelegramMessagesCursor parses the opaque token; an empty input is not
// an error (caller treats empty cursor as "first page"). Malformed input
// returns a non-nil error so the handler can map it to 422 CodeValidation.
func DecodeTelegramMessagesCursor(s string) (*TelegramMessagesCursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode telegram_messages cursor: %w", err)
	}
	var c TelegramMessagesCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("decode telegram_messages cursor: %w", err)
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return nil, errors.New("decode telegram_messages cursor: missing fields")
	}
	return &c, nil
}
