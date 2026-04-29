package telegram

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// startCommandPattern matches the canonical /start command Telegram emits
// when a deep-link is followed in private chat (`/start <payload>`) and the
// `/start@bot_username <payload>` form Telegram inserts in groups. Any
// payload (uuid or otherwise) is captured in group 2; absence of payload
// matches with empty group 2.
var startCommandPattern = regexp.MustCompile(`^/start(?:@\S+)?(?:\s+(.+))?$`)

// Dispatcher routes a single IncomingUpdate to the right handler. Today the
// only command is /start <uuid>; everything else goes to a fallback reply.
// Future commands (e.g. /status) plug in here without touching the runner.
type Dispatcher interface {
	Dispatch(ctx context.Context, update IncomingUpdate)
}

// StartCommandHandler is the narrow contract dispatcher needs to reach the
// /start handler — accept-interfaces / return-structs.
type StartCommandHandler interface {
	Handle(ctx context.Context, update IncomingUpdate, payload string)
}

// dispatcher implements Dispatcher. Send-only failures (e.g. SendMessage
// returning a Telegram-side error) are logged here once and then swallowed —
// the runner's offset advance after Dispatch makes one missed reply the price
// of forward progress; the user can re-issue /start to recover.
type dispatcher struct {
	client   Client
	start    StartCommandHandler
	messages Messages
	logger   logger.Logger
}

// NewDispatcher wires the dispatcher with its dependencies.
func NewDispatcher(client Client, start StartCommandHandler, messages Messages, log logger.Logger) Dispatcher {
	return &dispatcher{client: client, start: start, messages: messages, logger: log}
}

func (d *dispatcher) Dispatch(ctx context.Context, update IncomingUpdate) {
	// Trim ASCII / Unicode whitespace via unicode.IsSpace and lowercase to
	// make the routing case-insensitive. Telegram canonicalises commands
	// in lowercase but desktop clients sometimes pass through "/Start" —
	// we treat them the same. ZWNBSP (U+FEFF) is *not* considered space
	// by unicode.IsSpace and would not be stripped — anyone seeing such
	// input from real Telegram users would be evidence of a malformed
	// client, not an expected case.
	text := strings.TrimFunc(update.Text, unicode.IsSpace)
	lower := strings.ToLower(text)

	m := startCommandPattern.FindStringSubmatch(lower)
	if m == nil {
		d.reply(ctx, update.ChatID, d.messages.Fallback())
		return
	}
	payload := strings.TrimSpace(m[1])
	if payload == "" {
		d.reply(ctx, update.ChatID, d.messages.StartNoPayload())
		return
	}
	// Pull the payload from the original (non-lowered) text so UUIDs with
	// uppercase hex (technically valid but we still parse-check downstream)
	// arrive in their original casing.
	originalMatch := startCommandPattern.FindStringSubmatch(text)
	if len(originalMatch) > 1 {
		if op := strings.TrimSpace(originalMatch[1]); op != "" {
			payload = op
		}
	}
	d.start.Handle(ctx, update, payload)
}

func (d *dispatcher) reply(ctx context.Context, chatID int64, text string) {
	if err := d.client.SendMessage(ctx, chatID, text); err != nil {
		// chatID is a Telegram identifier (numeric, public-ish), not PII per
		// security.md, so logging it here is safe. Reply text is templated
		// copy from messages.go — also non-PII.
		d.logger.Warn(ctx, "telegram dispatcher reply failed", "chat_id", chatID, "error", err)
	}
}
