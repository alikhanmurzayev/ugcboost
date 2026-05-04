// Package telegram — E2E тесты Telegram-бота.
//
// Тесты используют POST /test/telegram/message: ручка собирает synthetic
// Update, прогоняет через тот же in-process handler, что и production
// long polling, и возвращает синхронные ответы, перехваченные in-memory
// per-call SpyOnlySender. **Внимание:** этот эндпоинт ловит только
// синхронные handler-reply'и (error-ветки) — успешный link больше не
// шлёт sync-reply, welcome уходит fire-and-forget через Notifier и
// читается через GET /test/telegram/sent (chunk 9 рефактор).
//
// TestTelegramLink покрывает основной поток привязки. С-IG happy-path:
// заявка submitted с Instagram → /start → INSERT link + audit, отсутствие
// sync-reply, welcome через /test/telegram/sent с подстрокой `UGC-` и
// URL `https://ig.me/m/ugc_boost`. Без-IG happy-path: заявка только с
// TikTok → welcome без `UGC-` и `ig.me`, текст «Скоро сообщим здесь
// результаты отбора».
// Idempotent re-link: тот же TG жмёт /start ещё раз → 2 идентичных
// welcome-записи. «Чужой Telegram» — синхронный sync-reply
// «уже связана с другим Telegram», 0 новых записей в spy.
//
// TestTelegramFallback покрывает все ветки, не связанные с успешным link.
// /start без payload и /start с битой ссылкой получают универсальный
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
//
// E2E test-mode contract (см. spec-creator-bot-notify-foundation):
// `/test/telegram/sent` ловит то, что бэк попытался отправить (params,
// ChatID, ReplyMarkup) — не факт доставки. В TeeSender-режиме (mock=false
// + EnableTestEndpoints) реальный bot.SendMessage зовётся, на синтетических
// chat_id всегда падает; spy записывает params + Err. Err намеренно НЕ
// ассертим — тесты verify outbound params, не факт доставки.
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
	replyApplicationAlreadyLnk = "Эта заявка уже связана с другим Telegram. Если это ошибка — обратитесь в поддержку"

	// welcomeNoIGText дублирует строку из internal/telegram/notifier.go.
	// Дублирование намеренное: e2e-пакет — отдельный модуль, и assert-by-equality
	// требует, чтобы любое изменение копирайта одновременно ломало тест.
	welcomeNoIGText = "Здравствуйте! 👋\n\n" +
		"Мы получили вашу заявку. Скоро сообщим здесь результаты отбора ✅"
)

// welcomeWithIGText собирает welcome-сообщение для IG-варианта, подставляя
// реальный verification-код. Дублирует шаблон из internal/telegram/notifier.go
// по тем же соображениям, что и welcomeNoIGText. Любое изменение копирайта
// должно одновременно сломать тест.
func welcomeWithIGText(verificationCode string) string {
	return "Здравствуйте! 👋\n\n" +
		"Мы получили вашу заявку.\n" +
		"Подтвердите, пожалуйста, что вы действительно владеете указанным аккаунтом Instagram:\n\n" +
		"1. Скопируйте код:\n\n" +
		"   <pre>" + verificationCode + "</pre>\n\n" +
		"2. Откройте Direct и отправьте его нам:\n\n" +
		"   https://ig.me/m/ugc_boost"
}

// validRequestWithIG mirrors the helper from creator_application package
// without sharing types — keeps the two e2e packages independent. Carries
// an Instagram social so the welcome variant exercises the with-IG branch.
func validRequestWithIG(iin string) apiclient.CreatorApplicationSubmitRequest {
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

// validRequestNoIG drops the Instagram social — TikTok only — so the welcome
// variant exercises the no-IG branch.
func validRequestNoIG(iin string) apiclient.CreatorApplicationSubmitRequest {
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
			{Platform: apiclient.Tiktok, Handle: "aidana_tt_" + iin[7:]},
		},
		AcceptedAll: true,
	}
}

func submitWithRequest(t *testing.T, req apiclient.CreatorApplicationSubmitRequest) string {
	t.Helper()
	c := testutil.NewAPIClient(t)
	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.ApplicationId.String()
	testutil.RegisterCreatorApplicationCleanup(t, id)
	return id
}

// requireNoSyncReply asserts the test endpoint returned an empty replies list.
// Welcome on success-link is async via /test/telegram/sent, not a sync reply.
func requireNoSyncReply(t *testing.T, replies []testclient.TelegramReply) {
	t.Helper()
	require.Empty(t, replies, "success /start must produce no synchronous reply (welcome is async via Notifier)")
}

func singleReply(t *testing.T, replies []testclient.TelegramReply, chatID int64) string {
	t.Helper()
	require.Len(t, replies, 1)
	require.Equal(t, chatID, replies[0].ChatId)
	return replies[0].Text
}

func TestTelegramLink(t *testing.T) {
	t.Parallel()

	t.Run("with-IG: success creates link, sends welcome with UGC code and Direct URL", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitWithRequest(t, validRequestWithIG(testutil.UniqueIIN()))
		code := testutil.GetCreatorApplicationVerificationCode(t, appID)

		upd := testutil.DefaultTelegramUpdate(t)
		upd.Text = "/start " + appID
		since := time.Now().UTC()
		requireNoSyncReply(t, testutil.SendTelegramUpdate(t, tc, upd))

		sent := testutil.WaitForTelegramSent(t, upd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, sent, 1)
		require.Equal(t, upd.UserID, sent[0].ChatId)
		require.Equal(t, welcomeWithIGText(code), sent[0].Text)
		require.Nil(t, sent[0].WebAppUrl, "chunk-9 messages never carry a WebApp button")

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

	t.Run("no-IG: success sends generic welcome, no UGC code, no ig.me", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitWithRequest(t, validRequestNoIG(testutil.UniqueIIN()))

		upd := testutil.DefaultTelegramUpdate(t)
		upd.Text = "/start " + appID
		since := time.Now().UTC()
		requireNoSyncReply(t, testutil.SendTelegramUpdate(t, tc, upd))

		sent := testutil.WaitForTelegramSent(t, upd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, sent, 1)
		require.Equal(t, welcomeNoIGText, sent[0].Text)
		require.Nil(t, sent[0].WebAppUrl)
	})

	t.Run("idempotent repeat from same TG produces two welcome records", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitWithRequest(t, validRequestWithIG(testutil.UniqueIIN()))
		upd := testutil.DefaultTelegramUpdate(t)
		upd.Text = "/start " + appID
		since := time.Now().UTC()

		requireNoSyncReply(t, testutil.SendTelegramUpdate(t, tc, upd))
		// Wait for the first welcome before issuing the second /start so
		// the count assertion below cannot race the first goroutine.
		_ = testutil.WaitForTelegramSent(t, upd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		requireNoSyncReply(t, testutil.SendTelegramUpdate(t, tc, upd))

		sent := testutil.WaitForTelegramSent(t, upd.UserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 2,
		})
		require.Len(t, sent, 2, "idempotent re-link must trigger a second welcome")
		expected := welcomeWithIGText(testutil.GetCreatorApplicationVerificationCode(t, appID))
		require.Equal(t, expected, sent[0].Text)
		require.Equal(t, expected, sent[1].Text)

		// Audit entry exists exactly once — repeated /start must not produce a
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

	t.Run("application already linked to a different TG: sync reply, intruder gets no welcome", func(t *testing.T) {
		t.Parallel()
		tc := testutil.NewTestClient(t)
		appID := submitWithRequest(t, validRequestWithIG(testutil.UniqueIIN()))

		first := testutil.DefaultTelegramUpdate(t)
		first.Text = "/start " + appID
		firstSince := time.Now().UTC()
		requireNoSyncReply(t, testutil.SendTelegramUpdate(t, tc, first))
		// Wait for the first welcome so the intruder branch starts from a
		// known steady state. firstSince is captured immediately before
		// SendTelegramUpdate so the spy filter cannot drift onto records
		// from a parallel test that happens to share a chat id.
		_ = testutil.WaitForTelegramSent(t, first.UserID, testutil.TelegramSentOptions{
			Since:       firstSince,
			ExpectCount: 1,
		})

		intruder := testutil.DefaultTelegramUpdate(t)
		intruder.Text = "/start " + appID
		intruderSince := time.Now().UTC()
		require.Equal(t, replyApplicationAlreadyLnk,
			singleReply(t, testutil.SendTelegramUpdate(t, tc, intruder), intruder.ChatID))

		// No new welcome for the intruder (notify is gated on commit-success).
		// `EnsureNoNewTelegramSent` polls /test/telegram/sent for a window and
		// fails the test if any record appears — a non-empty result fails the
		// helper, an empty result over the full window passes.
		testutil.EnsureNoNewTelegramSent(t, intruder.UserID, intruderSince, 1*time.Second)

		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		detailResp, err := adminClient.GetCreatorApplicationWithResponse(context.Background(),
			mustParseUUID(t, appID), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.NotNil(t, detailResp.JSON200)
		require.NotNil(t, detailResp.JSON200.Data.TelegramLink)
		require.Equal(t, first.UserID, detailResp.JSON200.Data.TelegramLink.TelegramUserId,
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
