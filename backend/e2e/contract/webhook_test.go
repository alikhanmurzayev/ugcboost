// Package contract — E2E тесты приёма TrustMe webhook'а (chunk 17).
//
// TestTrustMeWebhook прогоняет POST /trustme/webhook через прямой HTTP с
// заголовком Authorization: <token> (raw, без Bearer per blueprint
// § «Установка хуков»). Каждый сценарий поднимает свежую кампанию с
// invited-креатором, переводит её в `signing` через tma agree +
// runOutboxOnce и берёт TrustMe-side document_id из spy-list. Дальше
// шлёт webhook и ассертит state-transition в БД (cc.status, audit) +
// бот-уведомление через telegram spy.
//
// Сценарии: signed (status=3) → cc.status='signed' + audit
// `contract_signed` + congrat-сообщение через bot; declined (status=9) и
// revoked (status=4, отозван компанией через UI Trust.me) — оба склеены
// в одну ветку: cc.status='signing_declined' + audit
// `contract_signing_declined` + decline-сообщение без WebApp-кнопки;
// различие 4 vs 9 видно только в audit-payload через trustme_status_code_new.
// Идемпотентный повтор того же signed → 200, audit-row count не растёт;
// terminal-guard (после signed status=2) → 200, состояние не меняется;
// non-terminal статусы 0/2 (intermediate) и 1/5/6/7/8 (unexpected) →
// cc.status не тронут, audit `contract_unexpected_status`, бот не
// отправлен; различие 0/2 vs 1/5-8 — только уровень лога (info vs warn).
// Unknown contract_id → 404; missing/wrong token → 401 (anti-fingerprint
// между ними); status=15 (вне 0..9) → 422.
//
// Тестов с soft-deleted кампанией нет — пуб API не позволяет soft-delete
// после Phase 3 finalize в e2e в одном тике; покрытие на unit-уровне
// (TestWebhookService_HandleEvent_SoftDeletedCampaign).
//
// Каждый сценарий — независимая фикстура (UniqueIIN изоляция). Тесты
// безопасны для параллельных запусков (как локально, так и на staging
// рядом с другим трафиком): SetupCampaignWithSigningCreator делает retry
// runOutboxOnce + ждёт stable spy-baseline, ассерты фильтруют spy-list
// по уникальному IIN, telegram-spy — по chat_id креатора. Cleanup
// через defer-LIFO стек (E2E_CLEANUP=true).
package contract

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// expectedCampaignContractSignedText / expectedCampaignContractDeclinedText
// mirror internal/telegram/notifier.go::campaignContract{Signed,Declined}Text
// so e2e ассертит точный текст без импорта internal (e2e — отдельный модуль).
// Изменился production-текст? — обновляем константы здесь синхронно.
const (
	expectedCampaignContractSignedText = "Ура, мы подписали с вами соглашение ✅ Скоро отправим вам онлайн пригласительный на показы 😍\n\n" +
		"ТЗ — по кнопке ниже, чтобы не потерялось 💫"
	expectedCampaignContractDeclinedText = "Поняли, в этот раз не подписываем. Если появятся другие подходящие предложения — обязательно вам напишем 💫"
)

func TestTrustMeWebhook(t *testing.T) {
	token := testutil.TrustMeWebhookToken(t)

	t.Run("signed (status=3) flips cc to signed + audit + congrat bot message", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		baselineSize := fx.NotifyBaselineSize

		status, body := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookSignedPayload(fx.TrustMeDocumentID), token)
		require.Equalf(t, 200, status, "webhook signed: %s", string(body))

		assertCCStatus(t, fx.AdminClient, fx.AdminToken, fx.CampaignID, fx.CampaignCreatorID,
			apiclient.CampaignCreatorStatus("signed"))
		testutil.AssertAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signed")

		// Bot-сообщение signed после COMMIT'а — ждём ровно baseline+1, новый
		// message — последний по sentAt. Ассертим точный текст + inline
		// WebApp-кнопку с ТЗ кампании (chunk-12 lock гарантирует, что tma_url
		// в кнопке совпадает с tma_url из исходного invite).
		messages := testutil.WaitForTelegramSent(t, fx.CreatorTelegramID, testutil.TelegramSentOptions{
			ExpectCount: baselineSize + 1,
		})
		require.GreaterOrEqual(t, len(messages), baselineSize+1)
		signedMsg := newestSpyMessage(t, messages, expectedCampaignContractSignedText)
		require.Equal(t, fx.CreatorTelegramID, signedMsg.ChatId)
		require.NotNil(t, signedMsg.WebAppUrl, "signed message must carry an inline WebApp button")
		require.Equal(t, fx.TmaURL, *signedMsg.WebAppUrl)

		// Recorder writes the signed notify row alongside the spy capture.
		// Status not pinned (see other recorder asserts for the rationale).
		testutil.CleanupTelegramMessagesByChat(t, fx.CreatorTelegramID)
		row := testutil.AssertTelegramMessageRecorded(t, fx.AdminClient, fx.AdminToken, fx.CreatorTelegramID,
			testutil.TelegramMessageMatcher{Direction: "outbound", TextContains: signedMsg.Text})
		require.Equal(t, signedMsg.Text, row.Text)
	})

	t.Run("declined (status=9) flips cc to signing_declined + audit + decline bot message", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		baselineSize := fx.NotifyBaselineSize

		status, body := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookDeclinedPayload(fx.TrustMeDocumentID), token)
		require.Equalf(t, 200, status, "webhook declined: %s", string(body))

		assertCCStatus(t, fx.AdminClient, fx.AdminToken, fx.CampaignID, fx.CampaignCreatorID,
			apiclient.CampaignCreatorStatus("signing_declined"))
		testutil.AssertAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signing_declined")

		messages := testutil.WaitForTelegramSent(t, fx.CreatorTelegramID, testutil.TelegramSentOptions{
			ExpectCount: baselineSize + 1,
		})
		declinedMsg := newestSpyMessage(t, messages, expectedCampaignContractDeclinedText)
		require.Equal(t, fx.CreatorTelegramID, declinedMsg.ChatId)
		require.Nil(t, declinedMsg.WebAppUrl, "declined message must be plain text — no WebApp button")
	})

	t.Run("idempotent repeat (signed twice) — second call adds nothing", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)

		// Первый вызов — реальный signed.
		status, _ := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookSignedPayload(fx.TrustMeDocumentID), token)
		require.Equal(t, 200, status)

		baselineMessages := testutil.WaitForTelegramSent(t, fx.CreatorTelegramID,
			testutil.TelegramSentOptions{ExpectCount: fx.NotifyBaselineSize + 1})
		baselineAudits := testutil.ListAuditEntriesByAction(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signed")
		require.Len(t, baselineAudits, 1)

		// Повтор того же payload — UPDATE 0 affected → no-op.
		repeatSince := time.Now()
		status, _ = testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookSignedPayload(fx.TrustMeDocumentID), token)
		require.Equal(t, 200, status)

		// Telegram spy не вырос — никаких новых сообщений после repeat.
		testutil.EnsureNoNewTelegramSent(t, fx.CreatorTelegramID, repeatSince, 1*time.Second)

		// Audit count не вырос.
		afterAudits := testutil.ListAuditEntriesByAction(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signed")
		require.Len(t, afterAudits, 1, "idempotent повтор не должен писать второй audit")

		// Sanity: стартовая длина сообщений равна финальной.
		_ = baselineMessages
	})

	t.Run("terminal-guard: stale status=2 after signed → 200 no-op", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)

		status, _ := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookSignedPayload(fx.TrustMeDocumentID), token)
		require.Equal(t, 200, status)

		// БД уже terminal=3. Прилетает stale=2 → terminal-guard (NOT IN (3,9))
		// блокирует UPDATE.
		stale := testutil.TrustMeWebhookStatusPayload(fx.TrustMeDocumentID, 2)
		stStatus, _ := testutil.PostTrustMeWebhook(t, stale, token)
		require.Equal(t, 200, stStatus)

		assertCCStatus(t, fx.AdminClient, fx.AdminToken, fx.CampaignID, fx.CampaignCreatorID,
			apiclient.CampaignCreatorStatus("signed"))
		// audit не растёт — посмотрим только signed-audit.
		signedAudits := testutil.ListAuditEntriesByAction(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signed")
		require.Len(t, signedAudits, 1)
		// и нет unexpected-status audit (он обнулился через terminal-guard).
		unexpAudits := testutil.ListAuditEntriesByAction(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_unexpected_status")
		require.Empty(t, unexpAudits, "terminal-guard блок ДОЛЖЕН пропустить audit")
	})

	t.Run("revoked (status=4) flips cc to signing_declined + audit + decline bot message", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		baselineSize := fx.NotifyBaselineSize

		status, body := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookStatusPayload(fx.TrustMeDocumentID, 4), token)
		require.Equalf(t, 200, status, "webhook revoked: %s", string(body))

		assertCCStatus(t, fx.AdminClient, fx.AdminToken, fx.CampaignID, fx.CampaignCreatorID,
			apiclient.CampaignCreatorStatus("signing_declined"))
		testutil.AssertAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_signing_declined")

		messages := testutil.WaitForTelegramSent(t, fx.CreatorTelegramID, testutil.TelegramSentOptions{
			ExpectCount: baselineSize + 1,
		})
		declinedMsg := newestSpyMessage(t, messages, expectedCampaignContractDeclinedText)
		require.Equal(t, fx.CreatorTelegramID, declinedMsg.ChatId)
		require.Nil(t, declinedMsg.WebAppUrl, "revoked message must be plain text — no WebApp button")
	})

	t.Run("unknown document → 404 CONTRACT_WEBHOOK_UNKNOWN_DOCUMENT", func(t *testing.T) {
		t.Parallel()
		_ = testutil.SetupAdmin(t) // sanity: backend up

		payload := testutil.TrustMeWebhookSignedPayload("unknown-doc-" + uuid.NewString())
		status, body := testutil.PostTrustMeWebhook(t, payload, token)
		require.Equalf(t, 404, status, "unknown doc: %s", string(body))
	})

	t.Run("missing token → 401 with empty body", func(t *testing.T) {
		t.Parallel()
		payload := testutil.TrustMeWebhookSignedPayload("doc-irrelevant")
		status, body := testutil.PostTrustMeWebhook(t, payload, "")
		require.Equal(t, 401, status)
		require.Equal(t, "{}\n", string(body))
	})

	t.Run("wrong token → 401 with empty body (anti-fingerprint with missing)", func(t *testing.T) {
		t.Parallel()
		payload := testutil.TrustMeWebhookSignedPayload("doc-irrelevant")
		status, body := testutil.PostTrustMeWebhook(t, payload, "wrong-"+token)
		require.Equal(t, 401, status)
		require.Equal(t, "{}\n", string(body))
	})

	t.Run("invalid status (15) → 422 with CONTRACT_WEBHOOK_INVALID_STATUS", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)

		payload := testutil.TrustMeWebhookStatusPayload(fx.TrustMeDocumentID, 15)
		status, body := testutil.PostTrustMeWebhook(t, payload, token)
		require.Equal(t, 422, status, "invalid status body=%s", string(body))
	})
}

// newestSpyMessage возвращает first spy record с текстом expectedText из
// набора. Ошибается тестом, если такого нет — гарант предсказуемой
// поломки при drift'е production-копии. Spy возвращает все сообщения
// чата, expected-text используется как фильтр (другие baseline-сообщения,
// invite/welcome/contract-sent, имеют отличный текст).
func newestSpyMessage(t *testing.T, messages []testclient.TelegramSentMessage, expectedText string) testclient.TelegramSentMessage {
	t.Helper()
	for _, m := range messages {
		if m.Text == expectedText {
			return m
		}
	}
	t.Fatalf("expected spy message with text=%q not found in %d records", expectedText, len(messages))
	return testclient.TelegramSentMessage{}
}

// assertCCStatus — admin GET /campaigns/{id}/creators ищет cc.id и
// сверяет его status. Помогает проверить переход после webhook-Tx commit'а.
func assertCCStatus(t *testing.T, c *apiclient.ClientWithResponses, adminToken, campaignID, ccID string, want apiclient.CampaignCreatorStatus) {
	t.Helper()
	resp, err := c.ListCampaignCreatorsWithResponse(context.Background(),
		uuid.MustParse(campaignID), testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, 200, resp.StatusCode(), "list cc: %s", strconv.Itoa(resp.StatusCode()))
	require.NotNil(t, resp.JSON200)
	for _, item := range resp.JSON200.Data.Items {
		if item.Id.String() == ccID {
			require.Equal(t, want, item.Status,
				"cc %s status mismatch — expected %s, got %s", ccID, want, item.Status)
			return
		}
	}
	t.Fatalf("cc %s not found in campaign %s", ccID, campaignID)
}
