package testutil

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

var telegramTestIDCounter int64

// UniqueTelegramUserID returns a fresh int64 large enough to never collide
// with a real Telegram user id during a manual smoke through the live bot.
func UniqueTelegramUserID() int64 {
	const epoch int64 = 1 << 30
	delta := atomic.AddInt64(&telegramTestIDCounter, 1)
	return epoch + (time.Now().UnixNano()%(1<<20))*1024 + delta
}

// TelegramUpdate is what the e2e tests inject through POST /test/telegram/message.
type TelegramUpdate struct {
	ChatID    int64
	UserID    int64
	Text      string
	Username  *string
	FirstName *string
	LastName  *string
}

// DefaultTelegramUpdate returns an update with a fresh, parallel-safe user id
// (also used as chat id) and synthetic profile metadata. Tests usually
// override Text with the command they want to drive.
func DefaultTelegramUpdate(t *testing.T) TelegramUpdate {
	t.Helper()
	id := UniqueTelegramUserID()
	username := fmt.Sprintf("tg_%d", id)
	firstName := "Тест"
	lastName := "Креатор"
	return TelegramUpdate{
		ChatID:    id,
		UserID:    id,
		Text:      "/start",
		Username:  &username,
		FirstName: &firstName,
		LastName:  &lastName,
	}
}

// SendTelegramUpdate posts to the test endpoint and returns the captured replies.
// Any non-200 fails the test — the production response shape is the only path.
//
// Recorder writes an inbound row (and possibly a welcome outbound) for every
// private update we inject, so the helper registers a defer-time cleanup
// against this chat_id. Repeat calls with the same chat_id queue idempotent
// extra cleanups — DELETE with no rows is a no-op.
func SendTelegramUpdate(t *testing.T, c *testclient.ClientWithResponses, u TelegramUpdate) []testclient.TelegramReply {
	t.Helper()
	if u.ChatID == 0 {
		u.ChatID = u.UserID
	}
	CleanupTelegramMessagesByChat(t, u.ChatID)
	resp, err := c.SendTelegramMessageWithResponse(context.Background(), testclient.SendTelegramMessageJSONRequestBody{
		ChatId:    u.ChatID,
		UserId:    &u.UserID,
		Text:      u.Text,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Replies
}
