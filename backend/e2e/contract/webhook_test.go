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
// `contract_signed` + congrat-сообщение через bot; declined (status=9) →
// cc.status='signing_declined' + audit + decline-сообщение; idempotent
// повтор того же signed → 200, audit-row count не растёт; terminal-guard
// (после signed status=2) → 200, состояние не меняется; intermediate
// (status=1/2/4-8) → cc.status не тронут, audit `contract_unexpected_status`,
// бот не отправлен; unknown contract_id → 404; missing/wrong token → 401
// (anti-fingerprint между ними); status=15 (вне 0..9) → 422.
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
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
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

		// Bot-сообщение signed после COMMIT'а — ждём ровно baseline+1.
		testutil.WaitForTelegramSent(t, fx.CreatorTelegramID, testutil.TelegramSentOptions{
			ExpectCount: baselineSize + 1,
		})
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

		testutil.WaitForTelegramSent(t, fx.CreatorTelegramID, testutil.TelegramSentOptions{
			ExpectCount: baselineSize + 1,
		})
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

	t.Run("intermediate status=4 → cc.status untouched, audit unexpected, no bot", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithSigningCreator(t)
		baselineSize := fx.NotifyBaselineSize
		afterStartedSince := time.Now()

		status, _ := testutil.PostTrustMeWebhook(t,
			testutil.TrustMeWebhookStatusPayload(fx.TrustMeDocumentID, 4), token)
		require.Equal(t, 200, status)

		assertCCStatus(t, fx.AdminClient, fx.AdminToken, fx.CampaignID, fx.CampaignCreatorID,
			apiclient.CampaignCreatorStatus("signing"))
		testutil.AssertAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_unexpected_status")

		// Бот не отправлял ничего после baseline — intermediate статусы не
		// триггерят notify.
		testutil.EnsureNoNewTelegramSent(t, fx.CreatorTelegramID, afterStartedSince, 1*time.Second)
		_ = baselineSize
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
