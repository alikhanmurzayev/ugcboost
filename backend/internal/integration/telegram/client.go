// Package telegram is the bot integration: Client (real / noop / spy),
// Dispatcher, StartHandler, PollingRunner. The bot binds an incoming Telegram
// /start <applicationId> command to the matching creator application via
// CreatorApplicationTelegramService.
//
// Production replies and updates flow exclusively through the Client
// interface. The real implementation is a thin net/http wrapper around the
// Telegram Bot API (getUpdates + sendMessage). The noop implementation is
// used when the runner is intentionally disabled (no token, mock mode). The
// spy implementation is used by the e2e test endpoint to capture replies in
// memory and assert against them without hitting the live Telegram API.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// telegramAPIBase is the canonical Bot API host. Hardcoded — the only place
// in the codebase that talks to t.me. realClient overrides it via apiBase
// field in tests, but in production this is always api.telegram.org.
const telegramAPIBase = "https://api.telegram.org"

// IncomingUpdate is the slim, serialisable view of a Telegram message update
// the dispatcher passes around. The dispatcher / start-handler only ever read
// from this struct, never from raw Telegram API objects, so the production
// flow and the e2e test injection share the same code path verbatim.
type IncomingUpdate struct {
	UpdateID  int64
	ChatID    int64
	UserID    int64
	Text      string
	Username  *string
	FirstName *string
	LastName  *string
}

// Client is the abstraction the dispatcher / start-handler / runner depend on.
// Three implementations satisfy it (real / noop / spy) — see NewClient for
// selection logic.
type Client interface {
	// SendMessage sends a plain-text message to chatID. The implementation
	// must be safe for concurrent use; the dispatcher invokes it from goroutines.
	SendMessage(ctx context.Context, chatID int64, text string) error
	// GetUpdates fetches at most a batch of updates with offset and timeout
	// semantics from Bot API getUpdates. Returns empty slice on long-poll
	// timeout. Wraps any HTTP/network error with %w so the runner can log it.
	GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]IncomingUpdate, error)
}

// SentMessage is what the spy client records every time SendMessage is called.
// E2E tests drain it after dispatch to assert the bot replied with the
// expected text on the expected chat.
type SentMessage struct {
	ChatID int64
	Text   string
}

// Spy exposes the test-side draining surface the e2e test endpoint needs.
// Production callers never see it (returned as nil from NewClient outside
// test mode).
type Spy interface {
	Drain(chatID int64) []SentMessage
}

// NewClient picks the right Client implementation based on configuration.
//
//   - cfg.EnableTestEndpoints → spyClient (driven by /test/telegram/send-update;
//     polling runner is also disabled so the spy never sees GetUpdates).
//   - cfg.TelegramMock || cfg.TelegramBotToken == "" → noopClient (everything
//     no-op; Info-log on construction tells operators why the runner is
//     dormant).
//   - otherwise → realClient (HTTPS to api.telegram.org).
//
// The Spy return value is non-nil iff the chosen client is the spy — this
// keeps prod wiring honest (a non-nil spy in production would be a bug).
func NewClient(cfg *config.Config, log logger.Logger) (Client, Spy, error) {
	switch {
	case cfg.EnableTestEndpoints:
		spy := newSpyClient(log)
		return spy, spy, nil
	case cfg.TelegramMock || cfg.TelegramBotToken == "":
		return newNoopClient(log), nil, nil
	default:
		return newRealClient(cfg.TelegramBotToken, log), nil, nil
	}
}

// realClient talks to api.telegram.org via plain net/http. The Telegram Bot
// API is small enough for two endpoints (getUpdates / sendMessage) that a
// dedicated SDK is overkill — see backend-libraries.md.
type realClient struct {
	token   string
	apiBase string
	http    *http.Client
	logger  logger.Logger
}

func newRealClient(token string, log logger.Logger) *realClient {
	return &realClient{
		token:   token,
		apiBase: telegramAPIBase,
		http:    &http.Client{},
		logger:  log,
	}
}

// telegramResponse is the canonical envelope every Bot API method returns.
// We unmarshal the result lazily into a dedicated struct per call, so the
// generic envelope stays untyped here.
type telegramResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Result      json.RawMessage `json:"result"`
}

// telegramAPIError captures a non-OK Bot API reply (e.g. HTTP 409 "terminated
// by other getUpdates request" during a Dokploy rolling deploy). The runner
// logs the description and retries, so the error type is opaque on purpose.
type telegramAPIError struct {
	Code        int
	Description string
}

func (e *telegramAPIError) Error() string {
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
}

// invoke performs one Bot API call. Body is JSON-marshalled and parsed; the
// caller passes a pointer to a typed result struct that gets unmarshalled
// from .result on success. timeout-less ctx and cfg.TelegramPollingTimeout
// are both honoured by net/http via the request context.
func (c *realClient) invoke(ctx context.Context, method string, body any, out any) error {
	url := fmt.Sprintf("%s/bot%s/%s", c.apiBase, c.token, method)
	var bodyReader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("telegram %s marshal: %w", method, err)
		}
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return fmt.Errorf("telegram %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s do: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var env telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s decode: %w", method, err)
	}
	if !env.OK {
		return &telegramAPIError{Code: env.ErrorCode, Description: env.Description}
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("telegram %s decode result: %w", method, err)
		}
	}
	return nil
}

func (c *realClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.invoke(ctx, "sendMessage", map[string]any{
		"chat_id": chatID,
		"text":    text,
	}, nil)
}

// telegramRawUpdate mirrors the subset of the Bot API Update payload we need.
// We deliberately ignore message types other than plain text — anything else
// will surface as Text == "" and route to the dispatcher's fallback branch.
type telegramRawUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

func (c *realClient) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]IncomingUpdate, error) {
	body := map[string]any{
		"timeout": int(timeout.Seconds()),
	}
	if offset > 0 {
		body["offset"] = offset
	}
	var raw []telegramRawUpdate
	if err := c.invoke(ctx, "getUpdates", body, &raw); err != nil {
		return nil, err
	}
	out := make([]IncomingUpdate, 0, len(raw))
	for _, u := range raw {
		if u.Message == nil || u.Message.From == nil {
			// Updates without a message/from (channel posts, edits, callbacks)
			// are ignored — the bot only handles direct text commands.
			continue
		}
		incoming := IncomingUpdate{
			UpdateID: u.UpdateID,
			ChatID:   u.Message.Chat.ID,
			UserID:   u.Message.From.ID,
			Text:     u.Message.Text,
		}
		if u.Message.From.Username != "" {
			v := u.Message.From.Username
			incoming.Username = &v
		}
		if u.Message.From.FirstName != "" {
			v := u.Message.From.FirstName
			incoming.FirstName = &v
		}
		if u.Message.From.LastName != "" {
			v := u.Message.From.LastName
			incoming.LastName = &v
		}
		out = append(out, incoming)
	}
	return out, nil
}

// noopClient is used when the bot runner is intentionally disabled (no token
// or TELEGRAM_MOCK=true). SendMessage logs at DEBUG so dev runs do not flood
// stdout but operators can still trace what would have been sent.
type noopClient struct {
	logger logger.Logger
}

func newNoopClient(log logger.Logger) *noopClient {
	return &noopClient{logger: log}
}

func (c *noopClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	c.logger.Debug(ctx, "telegram noop SendMessage", "chat_id", chatID, "text_length", len(text))
	return nil
}

func (c *noopClient) GetUpdates(_ context.Context, _ int64, _ time.Duration) ([]IncomingUpdate, error) {
	return nil, nil
}

// spyClient buffers outgoing messages keyed by chat. The e2e test endpoint
// dispatches an injected update through the production dispatcher and then
// drains the buffer for the chat under test, returning every reply the
// production code attempted to send.
type spyClient struct {
	mu     sync.Mutex
	queue  map[int64][]SentMessage
	logger logger.Logger
}

func newSpyClient(log logger.Logger) *spyClient {
	return &spyClient{
		queue:  make(map[int64][]SentMessage),
		logger: log,
	}
}

func (c *spyClient) SendMessage(_ context.Context, chatID int64, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue[chatID] = append(c.queue[chatID], SentMessage{ChatID: chatID, Text: text})
	return nil
}

func (c *spyClient) GetUpdates(_ context.Context, _ int64, _ time.Duration) ([]IncomingUpdate, error) {
	// The polling runner is disabled in test mode — updates flow only via
	// the test endpoint. Returning an empty slice keeps the spy contract
	// clean if anything accidentally calls GetUpdates.
	return nil, nil
}

// Drain atomically returns and clears the buffered messages for chatID.
// Calls for a chat with no outstanding messages return nil.
func (c *spyClient) Drain(chatID int64) []SentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.queue[chatID]
	delete(c.queue, chatID)
	return msgs
}
