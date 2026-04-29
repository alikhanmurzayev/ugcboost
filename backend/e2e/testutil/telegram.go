package testutil

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// telegramTestIDCounter backs unique TG identifiers so parallel tests never
// collide on the UNIQUE constraint over telegram_user_id.
var telegramTestIDCounter int64

// uniqueTelegramID returns a fresh int64 identifier. The base is the lower
// 32 bits of UnixNano with a per-process counter added; the result fits
// comfortably inside Telegram's BIGINT range and stays well clear of any
// realistic Telegram user_id (10^10).
func uniqueTelegramID() int64 {
	const epoch int64 = 1 << 31 // 2_147_483_648 — clear of Telegram's real-id space
	delta := atomic.AddInt64(&telegramTestIDCounter, 1)
	return epoch + (time.Now().UnixNano()%(1<<24))*1024 + delta
}

// TelegramUpdateParams collects everything the test endpoint needs to compose
// a synthetic update. ChatID defaults to UserID (private chat) when zero.
type TelegramUpdateParams struct {
	UpdateID  int64
	ChatID    int64
	UserID    int64
	Text      string
	Username  *string
	FirstName *string
	LastName  *string
}

// DefaultTelegramUpdateParams returns a TelegramUpdateParams seeded with
// fresh, parallel-safe identifiers and Telegram metadata. Tests usually
// override `Text` with the command they want to drive; the rest is fine to
// keep as is.
func DefaultTelegramUpdateParams(t *testing.T) TelegramUpdateParams {
	t.Helper()
	id := uniqueTelegramID()
	username := "test_" + uniqueLabel(t)
	firstName := "Тестовый"
	lastName := "Креатор"
	return TelegramUpdateParams{
		UpdateID:  id,
		ChatID:    id,
		UserID:    id,
		Text:      "/start",
		Username:  &username,
		FirstName: &firstName,
		LastName:  &lastName,
	}
}

// uniqueLabel returns a short string suitable for grepping later (PII guard).
// Backed by a per-process counter so parallel tests never collide.
func uniqueLabel(t *testing.T) string {
	t.Helper()
	n := atomic.AddUint64(&counter, 1)
	return runID + "-" + uintToString(n)
}

// uintToString wraps strconv.Itoa for uint64 without importing strconv at the
// call sites — keeps the helper signature tight.
func uintToString(n uint64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = digits[n%10]
		n /= 10
	}
	return string(b[i:])
}

// SendTelegramUpdate POSTs to /test/telegram/send-update with the given
// parameters and returns the captured replies the dispatcher produced. Any
// non-200 response trips the test — the production response shape is the
// only valid path.
func SendTelegramUpdate(t *testing.T, c *testclient.ClientWithResponses, params TelegramUpdateParams) []testclient.TelegramReply {
	t.Helper()
	if params.ChatID == 0 {
		params.ChatID = params.UserID
	}
	resp, err := c.SendTelegramUpdateWithResponse(context.Background(), testclient.SendTelegramUpdateJSONRequestBody{
		UpdateId:  params.UpdateID,
		ChatId:    params.ChatID,
		UserId:    params.UserID,
		Text:      params.Text,
		Username:  params.Username,
		FirstName: params.FirstName,
		LastName:  params.LastName,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Replies
}
