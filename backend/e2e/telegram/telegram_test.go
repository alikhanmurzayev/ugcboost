// Package telegram — E2E тесты Telegram-бота.
//
// Тесты используют POST /test/telegram/message: ручка собирает synthetic
// Update, прогоняет через тот же in-process handler, что и production
// long polling, и возвращает ответы, перехваченные in-memory spy Sender.
//
// TestTelegramLink покрывает основной поток привязки Telegram-аккаунта к
// поданной заявке. Тест сам подаёт заявку через публичную ручку, шлёт
// /start <appID> от свежего synthetic-аккаунта и проверяет: бот ответил
// success-текстом; admin GET /creators/applications/{id} возвращает
// заполненный telegramLink с теми же user_id/username/first/last; в audit-логе
// появилась строка action=creator_application_link_telegram. Подсценарий
// idempotent повторно шлёт /start от того же TG и убеждается, что тот же
// успешный ответ возвращается без задвоения данных. Подсценарий «другая
// заявка занята другим Telegram» проверяет, что попытка чужого аккаунта
// привязаться к уже занятой заявке получает текст «уже связана с другим
// Telegram», а исходная привязка остаётся нетронутой.
//
// TestTelegramFallback покрывает все ветки, не связанные с успешной
// привязкой. /start без payload и /start с битой ссылкой получают универсальный
// текст «подайте заявку на ugcboost.kz»; синтаксически валидный, но
// несуществующий UUID получает «заявка не найдена»; любая другая команда
// (/help) и плоский текст также сворачиваются в fallback.
//
// TestSendTelegramMessage_ValidationErrors остаётся как guard над test-only
// эндпоинтом: пустой text → 422, без обращения к боту.
//
// Каждый тест создаёт свежие данные через бизнес-ручки. RegisterCleanup
// удаляет заявки в обратном порядке (LIFO) при E2E_CLEANUP=true; привязки
// уходят каскадом через ON DELETE CASCADE на application_id.
package telegram_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)
	return id
}

func newRandomUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	return id.String()
}

const (
	replyFallback              = "Здравствуйте! Чтобы продолжить, подайте заявку на ugcboost.kz"
	replyApplicationNotFound   = "Заявка не найдена. Подайте новую на ugcboost.kz"
	replyLinkSuccess           = "Здравствуйте! Заявка успешно связана с вашим Telegram. В ближайшее время в этом чате откроется мини-приложение со статусом обработки заявки"
	replyApplicationAlreadyLnk = "Эта заявка уже связана с другим Telegram. Если это ошибка — обратитесь в поддержку"
)

// validRequest mirrors the helper from creator_application package without
// sharing types — keeps the two e2e packages independent.
func validRequest(iin string) apiclient.CreatorApplicationSubmitRequest {
	middle := "Ивановна"
	return apiclient.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		Iin:        iin,
		Phone:      "+77001234567",
		City:       "almaty",
		Categories: []string{"beauty", "fashion"},
		Socials: []apiclient.SocialAccountInput{
			{Platform: apiclient.Instagram, Handle: "@aidana_" + iin[7:]},
			{Platform: apiclient.Tiktok, Handle: "aidana_tt_" + iin[7:]},
		},
		AcceptedAll: true,
	}
}

func submitApplication(t *testing.T) string {
	t.Helper()
	c := testutil.NewAPIClient(t)
	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(testutil.UniqueIIN()))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.ApplicationId.String()
	testutil.RegisterCreatorApplicationCleanup(t, id)
	return id
}

func singleReply(t *testing.T, replies []testclient.TelegramReply, chatID int64) string {
	t.Helper()
	require.Len(t, replies, 1)
	require.Equal(t, chatID, replies[0].ChatId)
	return replies[0].Text
}

func TestTelegramLink(t *testing.T) {
	t.Parallel()

	t.Run("success creates link, returns success text and writes audit", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitApplication(t)

		upd := testutil.DefaultTelegramUpdate(t)
		upd.Text = "/start " + appID
		require.Equal(t, replyLinkSuccess, singleReply(t, testutil.SendTelegramUpdate(t, tc, upd), upd.ChatID))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		resp, err := adminClient.GetCreatorApplicationWithResponse(context.Background(),
			mustParseUUID(t, appID), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		link := resp.JSON200.Data.TelegramLink
		require.NotNil(t, link, "telegramLink must be present after successful /start")
		require.Equal(t, upd.UserID, link.TelegramUserId)
		require.Equal(t, upd.Username, link.TelegramUsername)
		require.Equal(t, upd.FirstName, link.TelegramFirstName)
		require.Equal(t, upd.LastName, link.TelegramLastName)
		require.WithinDuration(t, time.Now(), link.LinkedAt, 2*time.Minute)

		testutil.AssertAuditEntry(t, adminClient, adminToken,
			"creator_application", appID, "creator_application_link_telegram")
	})

	t.Run("idempotent repeat from same TG returns success without changes", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitApplication(t)
		upd := testutil.DefaultTelegramUpdate(t)
		upd.Text = "/start " + appID

		first := testutil.SendTelegramUpdate(t, tc, upd)
		require.Equal(t, replyLinkSuccess, singleReply(t, first, upd.ChatID))
		second := testutil.SendTelegramUpdate(t, tc, upd)
		require.Equal(t, replyLinkSuccess, singleReply(t, second, upd.ChatID))

		// audit entry exists exactly once — repeated /start must not produce a
		// second link audit row.
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		entityType := "creator_application"
		listResp, err := adminClient.ListAuditLogsWithResponse(context.Background(),
			&apiclient.ListAuditLogsParams{EntityType: &entityType, EntityId: &appID},
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.NotNil(t, listResp.JSON200)
		count := 0
		for _, entry := range listResp.JSON200.Data.Logs {
			if entry.Action == "creator_application_link_telegram" {
				count++
			}
		}
		require.Equal(t, 1, count, "idempotent repeat must not produce a second audit row")
	})

	t.Run("application already linked to a different TG", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitApplication(t)

		first := testutil.DefaultTelegramUpdate(t)
		first.Text = "/start " + appID
		require.Equal(t, replyLinkSuccess, singleReply(t, testutil.SendTelegramUpdate(t, tc, first), first.ChatID))

		intruder := testutil.DefaultTelegramUpdate(t)
		intruder.Text = "/start " + appID
		require.Equal(t, replyApplicationAlreadyLnk, singleReply(t, testutil.SendTelegramUpdate(t, tc, intruder), intruder.ChatID))

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		resp, err := adminClient.GetCreatorApplicationWithResponse(context.Background(),
			mustParseUUID(t, appID), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.NotNil(t, resp.JSON200)
		require.NotNil(t, resp.JSON200.Data.TelegramLink)
		require.Equal(t, first.UserID, resp.JSON200.Data.TelegramLink.TelegramUserId,
			"original link must remain intact")
	})
}

func TestTelegramFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "no payload", text: "/start", want: replyFallback},
		{name: "invalid payload", text: "/start abracadabra", want: replyFallback},
		{name: "unknown command", text: "/help", want: replyFallback},
		{name: "plain text", text: "hi there", want: replyFallback},
		{name: "application not found", text: "/start " + newRandomUUID(t), want: replyApplicationNotFound},
	}

	tc := testutil.NewTestClient(t)
	for _, tc2 := range cases {
		tc2 := tc2
		t.Run(tc2.name, func(t *testing.T) {
			t.Parallel()
			upd := testutil.DefaultTelegramUpdate(t)
			upd.Text = tc2.text
			require.Equal(t, tc2.want, singleReply(t, testutil.SendTelegramUpdate(t, tc, upd), upd.ChatID))
		})
	}
}

func TestSendTelegramMessage_ValidationErrors(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	resp, err := tc.SendTelegramMessageWithResponse(context.Background(), testclient.SendTelegramMessageJSONRequestBody{
		ChatId: 1,
		Text:   "",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}
