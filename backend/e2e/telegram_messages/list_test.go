// Package telegram_messages — E2E тесты HTTP-поверхности GET /telegram-messages.
//
// TestListTelegramMessages проверяет admin-only ручку чтения хронологической
// ленты переписки бота по chat_id. Setup сеет 12 рядов через POST
// /test/seed-telegram-message для одного chat_id и затем гоняет keyset
// cursor пагинацию: первая страница на limit=5 отдаёт пять рядов в
// DESC-порядке по (created_at, id) и непустой nextCursor; повторный запрос
// с этим cursor возвращает следующие пять; последний — оставшиеся два и
// nextCursor=null. Пустой chat_id (без рядов) отвечает 200 {items:[],
// nextCursor:null} — этот кейс пинит инвариант «нет рядов → пустая
// страница, без ошибки».
//
// TestListTelegramMessagesAuth закрывает 401/403/422. Без токена ответ 401
// через стандартный middleware. brand_manager-токен ловит 403 FORBIDDEN —
// ручка для админа, и precedent CanRejectCreatorApplication из
// `authz/creator_application.go` гарантирует одинаковую форму 403 между
// admin-only endpoint'ами. Невалидные параметры (limit=0, limit=101, мусорный
// cursor) дают 422 VALIDATION_ERROR; отсутствие chatId — 422 через
// generated wrapper (required в OpenAPI).
//
// Сетап через testutil.Setup* — пользователи и бренды снимаются автоматом.
// Засеянные telegram_messages-ряды чистит CleanupTelegramMessagesByChat,
// который вызывает DELETE /test/telegram-messages?chatId=... LIFO-порядком.
package telegram_messages_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestListTelegramMessages(t *testing.T) {
	t.Parallel()

	t.Run("cursor pagination over 12 rows: 5+5+2 + nextCursor flips", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		testClient := testutil.NewTestClient(t)
		chatID := testutil.UniqueTelegramUserID()
		testutil.CleanupTelegramMessagesByChat(t, chatID)

		// Seed 12 inbound rows with monotonically increasing telegram_message_id
		// so the test can later sanity-check ordering. Postgres `now()` gives
		// per-row timestamps with at least microsecond resolution; ties (if any)
		// break on id DESC — both deterministic.
		for i := 1; i <= 12; i++ {
			testutil.SeedTelegramMessage(t, testClient, chatID, "inbound", "msg-text",
				testutil.WithTelegramMessageID(int64(i)))
		}

		resp1, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp1.StatusCode())
		require.NotNil(t, resp1.JSON200)
		require.Len(t, resp1.JSON200.Data.Items, 5)
		require.NotNil(t, resp1.JSON200.Data.NextCursor)
		require.NotEmpty(t, *resp1.JSON200.Data.NextCursor)

		// Rows are returned DESC by (created_at, id), so the freshest seed
		// (telegram_message_id == 12) appears first on the first page.
		require.NotNil(t, resp1.JSON200.Data.Items[0].TelegramMessageId)
		require.Equal(t, int64(12), *resp1.JSON200.Data.Items[0].TelegramMessageId)

		resp2, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5, Cursor: resp1.JSON200.Data.NextCursor},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp2.StatusCode())
		require.NotNil(t, resp2.JSON200)
		require.Len(t, resp2.JSON200.Data.Items, 5)
		require.NotNil(t, resp2.JSON200.Data.NextCursor)

		resp3, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5, Cursor: resp2.JSON200.Data.NextCursor},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp3.StatusCode())
		require.NotNil(t, resp3.JSON200)
		require.Len(t, resp3.JSON200.Data.Items, 2)
		require.Nil(t, resp3.JSON200.Data.NextCursor)

		// Pages are disjoint: seen ids across the three pages cover all 12.
		seen := make(map[string]bool)
		for _, r := range resp1.JSON200.Data.Items {
			seen[r.Id.String()] = true
		}
		for _, r := range resp2.JSON200.Data.Items {
			seen[r.Id.String()] = true
		}
		for _, r := range resp3.JSON200.Data.Items {
			seen[r.Id.String()] = true
		}
		require.Len(t, seen, 12, "every seeded row must appear exactly once across pages")
	})

	t.Run("empty chat: 200 with items=[] and nextCursor=null", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		emptyChat := testutil.UniqueTelegramUserID()

		resp, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: emptyChat, Limit: 5},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Empty(t, resp.JSON200.Data.Items)
		require.Nil(t, resp.JSON200.Data.NextCursor)
	})

	t.Run("outbound failed row carries status + error", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		testClient := testutil.NewTestClient(t)
		chatID := testutil.UniqueTelegramUserID()
		testutil.CleanupTelegramMessagesByChat(t, chatID)

		testutil.SeedTelegramMessage(t, testClient, chatID, "outbound", "hi",
			testutil.WithStatus("failed"),
			testutil.WithError("bot was blocked by the user"))

		resp, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Len(t, resp.JSON200.Data.Items, 1)
		row := resp.JSON200.Data.Items[0]
		require.Equal(t, apiclient.TelegramMessageDirection("outbound"), row.Direction)
		require.NotNil(t, row.Status)
		require.Equal(t, apiclient.TelegramMessageStatus("failed"), *row.Status)
		require.NotNil(t, row.Error)
		require.Contains(t, *row.Error, "bot was blocked")
	})
}

func TestListTelegramMessagesAuth(t *testing.T) {
	t.Parallel()

	t.Run("no token returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		chatID := testutil.UniqueTelegramUserID()
		resp, err := c.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("brand_manager token returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"TGMessagesMgr-"+testutil.UniqueEmail("tgmessages-mgr"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		chatID := testutil.UniqueTelegramUserID()

		resp, err := mgrClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5},
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("limit=0 returns 422", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		chatID := testutil.UniqueTelegramUserID()
		resp, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 0},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("limit=101 returns 422", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		chatID := testutil.UniqueTelegramUserID()
		resp, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 101},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("garbage cursor returns 422", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		chatID := testutil.UniqueTelegramUserID()
		badCursor := "not-base64-json"
		resp, err := adminClient.ListTelegramMessagesWithResponse(context.Background(),
			&apiclient.ListTelegramMessagesParams{ChatId: chatID, Limit: 5, Cursor: &badCursor},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})
}
