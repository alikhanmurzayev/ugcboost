package testutil

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// TelegramMessageMatcher filters telegram_messages rows by content.
// `Direction` is required ("inbound" / "outbound"); `TextContains` narrows
// candidate rows to ones whose text contains the substring; `Status` (for
// outbound) and `ErrorContains` further pin a specific delivery outcome.
type TelegramMessageMatcher struct {
	Direction     string
	TextContains  string
	Status        string
	ErrorContains string
}

// telegramMessagePollInterval keeps wall-time low while giving the
// post-commit Notifier / RecordingSender goroutine room to flush.
const telegramMessagePollInterval = 100 * time.Millisecond

// AssertTelegramMessageRecorded polls GET /telegram-messages?chatId until a
// row matching the matcher appears, then returns it. Fails the test on
// timeout (default 5s). Inbound recording is synchronous from the long-poll
// handler, but outbound recording sits behind Notifier.fire, so polling is
// the only correct way to observe both.
func AssertTelegramMessageRecorded(
	t *testing.T,
	c *apiclient.ClientWithResponses,
	adminToken string,
	chatID int64,
	m TelegramMessageMatcher,
) apiclient.TelegramMessage {
	t.Helper()
	require.NotEmpty(t, m.Direction, "matcher.Direction is required")

	deadline := time.Now().Add(5 * time.Second)
	for {
		rows := listTelegramMessages(t, c, adminToken, chatID)
		for _, r := range rows {
			if string(r.Direction) != m.Direction {
				continue
			}
			if m.TextContains != "" && !strings.Contains(r.Text, m.TextContains) {
				continue
			}
			if m.Status != "" {
				if r.Status == nil || string(*r.Status) != m.Status {
					continue
				}
			}
			if m.ErrorContains != "" {
				if r.Error == nil || !strings.Contains(*r.Error, m.ErrorContains) {
					continue
				}
			}
			return r
		}
		if time.Now().After(deadline) {
			for i, r := range rows {
				status := "<nil>"
				if r.Status != nil {
					status = string(*r.Status)
				}
				t.Logf("row[%d]: direction=%s status=%s textLen=%d text=%q",
					i, string(r.Direction), status, len(r.Text), r.Text)
			}
			t.Fatalf("no telegram_messages row matched %+v for chat %d in 5s; got %d rows",
				m, chatID, len(rows))
		}
		time.Sleep(telegramMessagePollInterval)
	}
}

// listTelegramMessages paginates the admin endpoint until the server stops
// emitting a nextCursor. A safety cap of 20 pages (≤ 2000 rows) protects
// against an accidental endless loop — beyond that we fail the test loudly,
// so silent matcher-misses cannot be confused with a recorder regression.
func listTelegramMessages(
	t *testing.T,
	c *apiclient.ClientWithResponses,
	adminToken string,
	chatID int64,
) []apiclient.TelegramMessage {
	t.Helper()
	const safetyPages = 20
	var out []apiclient.TelegramMessage
	var cursor *string
	for page := 0; ; page++ {
		if page >= safetyPages {
			t.Fatalf("listTelegramMessages: exceeded %d-page safety cap for chat %d (test fixture too large?)",
				safetyPages, chatID)
		}
		params := &apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 100}
		if cursor != nil {
			params.Cursor = cursor
		}
		resp, err := c.ListTelegramMessagesWithResponse(context.Background(), params, WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		out = append(out, resp.JSON200.Data.Items...)
		if resp.JSON200.Data.NextCursor == nil || *resp.JSON200.Data.NextCursor == "" {
			break
		}
		cursor = resp.JSON200.Data.NextCursor
	}
	return out
}

// SeedTelegramMessage drives POST /test/seed-telegram-message and returns the
// new row id. Direction must be "inbound" / "outbound"; text, telegramMessageId,
// telegramUsername, status, error are optional and forwarded as supplied.
func SeedTelegramMessage(
	t *testing.T,
	c *testclient.ClientWithResponses,
	chatID int64,
	direction string,
	text string,
	opts ...SeedTelegramMessageOption,
) string {
	t.Helper()
	body := testclient.SeedTelegramMessageJSONRequestBody{
		ChatId:    chatID,
		Direction: testclient.SeedTelegramMessageRequestDirection(direction),
		Text:      text,
	}
	for _, opt := range opts {
		opt(&body)
	}
	resp, err := c.SeedTelegramMessageWithResponse(context.Background(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id.String()
}

// SeedTelegramMessageOption mutates the request body before the call. Helpers
// below mirror the openapi-test fields without exposing the generated client
// type to test files.
type SeedTelegramMessageOption func(*testclient.SeedTelegramMessageJSONRequestBody)

// WithTelegramMessageID sets the optional telegram_message_id field.
func WithTelegramMessageID(id int64) SeedTelegramMessageOption {
	return func(b *testclient.SeedTelegramMessageJSONRequestBody) {
		b.TelegramMessageId = &id
	}
}

// WithTelegramUsername sets the optional telegram_username field.
func WithTelegramUsername(username string) SeedTelegramMessageOption {
	return func(b *testclient.SeedTelegramMessageJSONRequestBody) {
		b.TelegramUsername = &username
	}
}

// WithStatus sets the optional status field ("sent" / "failed").
func WithStatus(status string) SeedTelegramMessageOption {
	return func(b *testclient.SeedTelegramMessageJSONRequestBody) {
		s := testclient.SeedTelegramMessageRequestStatus(status)
		b.Status = &s
	}
}

// WithError sets the optional error field.
func WithError(msg string) SeedTelegramMessageOption {
	return func(b *testclient.SeedTelegramMessageJSONRequestBody) {
		b.Error = &msg
	}
}

// CleanupTelegramMessagesByChat registers a defer-time call to
// DELETE /test/telegram-messages?chatId=... so e2e tests can drop their
// fixtures without leaking rows across runs. Honors E2E_CLEANUP=false.
func CleanupTelegramMessagesByChat(t *testing.T, chatID int64) {
	t.Helper()
	client := NewTestClient(t)
	RegisterCleanup(t, func(ctx context.Context) error {
		resp, err := client.CleanupTelegramMessagesWithResponse(ctx,
			&testclient.CleanupTelegramMessagesParams{ChatId: chatID})
		if err != nil {
			return err
		}
		require.Equal(t, http.StatusNoContent, resp.StatusCode())
		return nil
	})
}
