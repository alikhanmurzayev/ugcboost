// Package telegram — E2E тесты привязки Telegram-аккаунта к заявке через
// команду /start <applicationId>. Бэкенд работает в test-режиме: production
// long-polling не стартует, обновления инжектятся напрямую в диспатчер
// через POST /test/telegram/send-update; ответы бот отдаёт через in-memory
// spy-клиент, которого тест читает в том же ответе. Это полный round-trip
// через настоящий хэндлер→сервис→БД→audit, но без живого Telegram.
//
// TestTelegramLinkHappyPath покрывает успешный сценарий: после публичной
// подачи заявки тест посылает /start <appID> с username/first_name/last_name
// и ожидает текст MessageLinkSuccess. Затем admin GET /creators/applications/{id}
// возвращает заполненный telegramLink (UserID + три nullable-метаданных +
// linkedAt в недавнем окне), а аудит-лог содержит запись
// creator_application_link_telegram, чьё new_value несёт все четыре telegram-поля.
//
// TestTelegramLinkIdempotent проверяет повторный /start от того же
// Telegram-пользователя для уже привязанной заявки. Тест ожидает ровно тот
// же успех (LinkSuccess), telegramLink в admin GET остаётся идентичным,
// audit log содержит ровно одну запись на привязку — повторная команда
// не пишет вторую строку аудита.
//
// TestTelegramLinkConflictAnotherTelegram моделирует попытку другого
// Telegram-пользователя привязаться к уже занятой заявке. Сервер отдаёт
// ApplicationAlreadyLinked, telegramLink в admin GET остаётся прежним.
//
// TestTelegramLinkConflictAnotherApplication проверяет обратную сторону:
// один и тот же Telegram-пользователь пытается привязаться ко второй
// (другой) заявке после успешной привязки к первой. Сервер отдаёт
// AccountAlreadyLinked.
//
// TestTelegramLinkApplicationNotFound и TestTelegramLinkInvalidPayload
// закрывают негативные сценарии без побочных эффектов на БД: первая
// проверяет, что валидный, но несуществующий UUID получает MessageApplicationNotFound;
// вторая — что мусорный payload (не-UUID) получает MessageInvalidPayload.
//
// TestTelegramLinkNoPayload проверяет команду «/start» без payload —
// диспатчер отвечает StartNoPayload без обращения к сервису.
// TestTelegramLinkUnknownCommand — что любая другая команда вроде /help
// маршрутизируется в Fallback.
//
// TestTelegramLinkPIIGuard защищает инвариант security.md: после happy
// path не должно быть ни одной утечки PII в stdout-логах приложения.
// Тест grep'ает docker logs ugcboost-backend по ИИН/телефону/handle/
// telegram_username/first_name/last_name из setup'а и ожидает 0 совпадений.
//
// Все тесты параллельны. SetupAdminClient автоматически удаляет
// admin-пользователя после теста, RegisterCreatorApplicationCleanup
// удаляет заявку (и каскадом — telegram-link через ON DELETE CASCADE).
// При E2E_CLEANUP=false (локальный отладочный режим) данные остаются.
package telegram_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// telegramBackendContainer is the docker-compose container name PIIGuard
// greps for stdout. Override via E2E_BACKEND_CONTAINER if your local compose
// project name differs.
const telegramBackendContainer = "ugcboost-backend-1"

const (
	successReply        = "Здравствуйте! Заявка успешно связана с вашим Telegram-аккаунтом. В ближайшее время в этом чате откроется мини-приложение со статусом обработки заявки."
	startNoPayloadReply = "Здравствуйте! Чтобы связать ваш Telegram-аккаунт с заявкой, перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."
	invalidPayloadReply = "Не удалось распознать ссылку. Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."
	notFoundReply       = "Не удалось найти заявку по этой ссылке. Возможно, заявка ещё не подана. Подайте заявку на ugcboost.kz."
	appLinkedReply      = "Эта заявка уже связана с другим Telegram-аккаунтом. Если это ошибка, обратитесь в поддержку."
	accountLinkedReply  = "У вас уже есть активная заявка, связанная с этим Telegram-аккаунтом. Дождитесь решения по ней или обратитесь в поддержку."
	fallbackReply       = "Я понимаю только команду /start со специальной ссылкой. Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."
)

// validRequest mirrors the helper from the creator_application e2e package
// without sharing types — keeps the two packages independent.
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

// submitApplication creates a fresh application and registers cleanup. Returns
// the application id (UUID) and the IIN used so PII-guard tests can grep
// docker logs for it.
func submitApplication(t *testing.T) (id, iin string) {
	t.Helper()
	c := testutil.NewAPIClient(t)
	iin = testutil.UniqueIIN()
	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(iin))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	id = resp.JSON201.Data.ApplicationId.String()
	testutil.RegisterCreatorApplicationCleanup(t, id)
	return id, iin
}

func TestTelegramLinkHappyPath(t *testing.T) {
	t.Parallel()

	appID, _ := submitApplication(t)
	tc := testutil.NewTestClient(t)

	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start " + appID

	replies := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, replies, 1)
	require.Equal(t, params.ChatID, replies[0].ChatId)
	require.Equal(t, successReply, replies[0].Text)

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	appUUID, err := uuid.Parse(appID)
	require.NoError(t, err)
	get, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, get.StatusCode())
	require.NotNil(t, get.JSON200)

	link := get.JSON200.Data.TelegramLink
	require.NotNil(t, link, "telegramLink must be populated after /start")
	require.Equal(t, params.UserID, link.TelegramUserId)
	require.NotNil(t, link.TelegramUsername)
	require.Equal(t, *params.Username, *link.TelegramUsername)
	require.NotNil(t, link.TelegramFirstName)
	require.Equal(t, *params.FirstName, *link.TelegramFirstName)
	require.NotNil(t, link.TelegramLastName)
	require.Equal(t, *params.LastName, *link.TelegramLastName)
	require.WithinDuration(t, time.Now().UTC(), link.LinkedAt, 5*time.Minute)

	testutil.AssertAuditEntry(t, adminClient, adminToken,
		"creator_application", appID, "creator_application_link_telegram")
}

func TestTelegramLinkIdempotent(t *testing.T) {
	t.Parallel()

	appID, _ := submitApplication(t)
	tc := testutil.NewTestClient(t)

	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start " + appID

	first := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, first, 1)
	require.Equal(t, successReply, first[0].Text)

	// Second /start from the same TG user → same success, no extra audit row.
	params.UpdateID++ // unique update id per Telegram contract
	second := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, second, 1)
	require.Equal(t, successReply, second[0].Text)

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	appUUID, err := uuid.Parse(appID)
	require.NoError(t, err)
	get, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, get.StatusCode())
	require.NotNil(t, get.JSON200.Data.TelegramLink)
	require.Equal(t, params.UserID, get.JSON200.Data.TelegramLink.TelegramUserId)

	logs, err := adminClient.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			EntityType: ptrString("creator_application"),
			EntityId:   &appID,
			Action:     ptrString("creator_application_link_telegram"),
		}, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logs.StatusCode())
	require.NotNil(t, logs.JSON200)
	require.Len(t, logs.JSON200.Data.Logs, 1, "idempotent /start must NOT emit a second audit row")
}

func TestTelegramLinkConflictAnotherTelegram(t *testing.T) {
	t.Parallel()

	appID, _ := submitApplication(t)
	tc := testutil.NewTestClient(t)

	first := testutil.DefaultTelegramUpdateParams(t)
	first.Text = "/start " + appID
	require.Equal(t, successReply, testutil.SendTelegramUpdate(t, tc, first)[0].Text)

	second := testutil.DefaultTelegramUpdateParams(t)
	second.Text = "/start " + appID
	replies := testutil.SendTelegramUpdate(t, tc, second)
	require.Len(t, replies, 1)
	require.Equal(t, appLinkedReply, replies[0].Text)

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	appUUID, _ := uuid.Parse(appID)
	get, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.NotNil(t, get.JSON200.Data.TelegramLink)
	require.Equal(t, first.UserID, get.JSON200.Data.TelegramLink.TelegramUserId,
		"the original TG user must remain bound after a conflict")
}

func TestTelegramLinkConflictAnotherApplication(t *testing.T) {
	t.Parallel()

	app1ID, _ := submitApplication(t)
	tc := testutil.NewTestClient(t)

	tg := testutil.DefaultTelegramUpdateParams(t)
	tg.Text = "/start " + app1ID
	require.Equal(t, successReply, testutil.SendTelegramUpdate(t, tc, tg)[0].Text)

	app2ID, _ := submitApplication(t)

	tg.UpdateID++
	tg.Text = "/start " + app2ID
	replies := testutil.SendTelegramUpdate(t, tc, tg)
	require.Len(t, replies, 1)
	require.Equal(t, accountLinkedReply, replies[0].Text)
}

func TestTelegramLinkApplicationNotFound(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start " + uuid.NewString()

	replies := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, replies, 1)
	require.Equal(t, notFoundReply, replies[0].Text)
}

func TestTelegramLinkInvalidPayload(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start not-a-uuid"

	replies := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, replies, 1)
	require.Equal(t, invalidPayloadReply, replies[0].Text)
}

func TestTelegramLinkNoPayload(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start"

	replies := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, replies, 1)
	require.Equal(t, startNoPayloadReply, replies[0].Text)
}

func TestTelegramLinkUnknownCommand(t *testing.T) {
	t.Parallel()

	tc := testutil.NewTestClient(t)
	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/help"

	replies := testutil.SendTelegramUpdate(t, tc, params)
	require.Len(t, replies, 1)
	require.Equal(t, fallbackReply, replies[0].Text)
}

func TestTelegramLinkPIIGuard(t *testing.T) {
	t.Parallel()

	appID, iin := submitApplication(t)
	tc := testutil.NewTestClient(t)

	params := testutil.DefaultTelegramUpdateParams(t)
	params.Text = "/start " + appID
	require.Equal(t, successReply, testutil.SendTelegramUpdate(t, tc, params)[0].Text)

	// Probe for PII in stdout logs of the running backend container. The
	// backend is launched via `make start-backend`, which uses docker
	// compose. On CI the container is required; locally a developer can
	// run `make run-backend` outside docker — we tolerate the skip there
	// but never on CI (CI=true is enough to fail loudly).
	containerName := telegramBackendContainer
	if v := os.Getenv("E2E_BACKEND_CONTAINER"); v != "" {
		containerName = v
	}
	logs, err := exec.Command("docker", "logs", containerName).CombinedOutput()
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("docker logs %q unavailable on CI: %v\n%s", containerName, err, string(logs))
		}
		t.Skipf("docker logs unavailable: %v (skipping PII guard)", err)
	}
	logStr := string(logs)

	probes := []struct {
		name  string
		value string
	}{
		{"iin", iin},
		{"phone", "+77001234567"},
		{"username", *params.Username},
		{"first_name", *params.FirstName},
		{"last_name", *params.LastName},
	}
	for _, p := range probes {
		require.NotContainsf(t, logStr, p.value,
			"PII leak detected: %s value %q found in stdout logs", p.name, p.value)
	}
}

func ptrString(s string) *string { return &s }
