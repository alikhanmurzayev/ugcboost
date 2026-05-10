package testutil

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// ExpectedCampaignContractSentText mirrors internal/telegram/notifier.go::
// campaignContractSentText so e2e ассертит точный Phase-3 ContractSent
// текст без импорта internal. Изменился production-текст — обновляем здесь.
const ExpectedCampaignContractSentText = "Мы отправили вам соглашение на подпись по СМС на номер телефона, указанный при регистрации 📄\n\n" +
	"Перейдите по ссылке из СМС и подпишите соглашение\n\n" +
	"Если есть вопросы, можете обратиться к @iskarova"

// RunTrustMeOutboxOnce синхронно прогоняет один тик ContractSenderService —
// гейтит /test/trustme/run-outbox-once. e2e webhook scenario использует это
// чтобы перевести cc.status в `signing` и получить TrustMe document_id в
// spy-list без ожидания @every 10s крон-тика.
func RunTrustMeOutboxOnce(t *testing.T) {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.TrustMeRunOutboxOnceWithResponse(context.Background())
	require.NoError(t, err)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode(),
		"RunTrustMeOutboxOnce: %d %s", resp.StatusCode(), string(resp.Body))
}

// TrustMeSpyList — обёртка над /test/trustme/spy-list для e2e suites вне
// пакета `contract`.
func TrustMeSpyList(t *testing.T) []testclient.TrustMeSentRecord {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.TrustMeSpyListWithResponse(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Items
}

// FindTrustMeSpyByIIN ищет в spy-list первую запись по IIN. Тесты сеют
// уникальный IIN через UniqueIIN, поэтому ровно одна совпадает. Использует
// IIN потому что fixture (TmaCampaignFixture.CreatorIIN) гарантирует
// уникальность между параллельными тестами; document_id и contract_id
// прорастают только после Phase 3 finalize, не годятся для фильтра.
func FindTrustMeSpyByIIN(t *testing.T, iin string) testclient.TrustMeSentRecord {
	t.Helper()
	if rec, ok := findSpyByIIN(t, iin); ok {
		return rec
	}
	t.Fatalf("no TrustMe spy record found for IIN=%s", iin)
	return testclient.TrustMeSentRecord{}
}

// findSpyByIIN — non-fatal вариант поиска для retry-логики.
func findSpyByIIN(t *testing.T, iin string) (testclient.TrustMeSentRecord, bool) {
	t.Helper()
	for _, r := range TrustMeSpyList(t) {
		if r.Iin == iin {
			return r, true
		}
	}
	return testclient.TrustMeSentRecord{}, false
}

// SigningCampaignFixture расширяет TmaCampaignFixture (`signing` cc + spy
// document_id), нужно chunk-17 e2e webhook'у. ContractID — внутренний
// contracts.id (UUID), TrustMeDocumentID — TrustMe-side ID, который шлётся
// в payload webhook'а как `contract_id`.
type SigningCampaignFixture struct {
	TmaCampaignFixture
	ContractID         string
	TrustMeDocumentID  string
	CreatorTelegramID  int64
	NotifyBaselineSize int
}

// SetupCampaignWithSigningCreator готовит cc.status='signing' через полный
// flow: SetupCampaignWithInvitedCreator → tma agree → один тик outbox-
// worker'а → читает TrustMe document_id из spy-list. Возвращает обогащённый
// fixture для chunk-17 webhook сценариев.
//
// NotifyBaselineSize — кол-во telegram-сообщений, которые spy уже записал
// до webhook scenario'а (invite + contract-sent после Phase 3). Тесты
// используют его как since-baseline при WaitForTelegramSent / EnsureNoNew.
func SetupCampaignWithSigningCreator(t *testing.T) SigningCampaignFixture {
	t.Helper()
	fx := SetupCampaignWithInvitedCreator(t)
	initData := SignInitData(t, fx.TelegramUserID, SignInitDataOpts{})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		BaseURL+"/tma/campaigns/"+fx.SecretToken+"/agree", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "tma "+initData)
	resp, err := HTTPClient(nil).Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equalf(t, http.StatusOK, resp.StatusCode, "tma agree: %s", string(body))

	// claimBatchSize=4 — параллельные тесты могут конкурировать за один тик.
	// Ретраим runOutboxOnce пока наш IIN не попадёт в spy (или 5 раз).
	var rec testclient.TrustMeSentRecord
	for attempt := 0; attempt < 5; attempt++ {
		RunTrustMeOutboxOnce(t)
		if found, ok := findSpyByIIN(t, fx.CreatorIIN); ok && found.DocumentId != nil && *found.DocumentId != "" {
			rec = found
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	require.NotEmpty(t, rec.AdditionalInfo, "TrustMe spy must capture our IIN after retried outbox ticks")
	require.NotNil(t, rec.DocumentId)
	require.NotEmpty(t, *rec.DocumentId)

	// Дожидаемся всех baseline-уведомлений креатору и стабилизируем их
	// перед возвратом, иначе late-arriving retry attempt'ы попадают в
	// окно EnsureNoNewTelegramSent последующего webhook-сценария.
	//
	// Истоки baseline-сообщений:
	//   1. application_approved — async после admin approve в SetupApprovedCreator.
	//      ВАЖНО: RegisterTelegramSpyFakeChat вызывается ПОСЛЕ approve, так
	//      что первая попытка SendMessage уходит в реальный bot и фейлится
	//      (404 chat not found), retry через ~1s даёт успех. Spy записывает
	//      ОБЕ попытки.
	//   2. campaignInvite — после A4 notify в SetupCampaignWithInvitedCreator
	//      (chat уже fake → одна запись).
	//   3. campaignContractSent — после Phase 3 finalize outbox-worker'а.
	//
	// Стабилизация: WaitForTelegramSent даёт нижнюю границу (≥3), затем
	// `waitSpyStable` повторно опрашивает spy и убеждается, что count не
	// растёт N раз подряд — late retries application_approved попадают в
	// этот pre-return период.
	notifyBaseline := WaitForTelegramSent(t, fx.TelegramUserID,
		TelegramSentOptions{ExpectCount: 3, Timeout: 10 * time.Second})
	stableBaseline := waitSpyStable(t, fx.TelegramUserID, len(notifyBaseline), 5*time.Second)

	// Phase 3 contract-sent проверяется здесь — every contract e2e сценарий
	// проходит через эту фикстуру, поэтому drift production-текста или
	// случайное добавление inline-кнопки сразу ломает любой контрактный тест.
	finalBaseline := currentSpyForChat(t, fx.TelegramUserID)
	contractSentMsg, found := findSpyMessageByText(finalBaseline, ExpectedCampaignContractSentText)
	require.True(t, found, "Phase 3 contract-sent message not found in baseline; checked %d records", len(finalBaseline))
	require.Equal(t, fx.TelegramUserID, contractSentMsg.ChatId)
	require.Nil(t, contractSentMsg.WebAppUrl, "contract-sent message must be plain text — no WebApp keyboard")

	return SigningCampaignFixture{
		TmaCampaignFixture: fx,
		ContractID:         rec.AdditionalInfo, // contracts.id (наш UUID)
		TrustMeDocumentID:  *rec.DocumentId,
		CreatorTelegramID:  fx.TelegramUserID,
		NotifyBaselineSize: stableBaseline,
	}
}

// findSpyMessageByText returns the first spy record matching expectedText.
// Helper for e2e assertions that filter by exact production copy without
// caring about ordering.
func findSpyMessageByText(messages []testclient.TelegramSentMessage, expectedText string) (testclient.TelegramSentMessage, bool) {
	for _, m := range messages {
		if m.Text == expectedText {
			return m, true
		}
	}
	return testclient.TelegramSentMessage{}, false
}

// waitSpyStable polls the telegram spy until the recorded message count for
// chatID stays unchanged for `stableFor` consecutive 200ms ticks (or until
// the global timeout). Used after WaitForTelegramSent to absorb late retry
// attempts that spy still records (e.g. real-bot 404 + retry success after
// fake-chat registration). Returns the final stable count.
func waitSpyStable(t *testing.T, chatID int64, initialCount int, stableFor time.Duration) int {
	t.Helper()
	const tick = 200 * time.Millisecond
	deadline := time.Now().Add(stableFor + 10*time.Second)
	stableTicks := 0
	requiredTicks := int(stableFor / tick)
	if requiredTicks < 1 {
		requiredTicks = 1
	}
	last := initialCount
	for time.Now().Before(deadline) {
		time.Sleep(tick)
		now := len(currentSpyForChat(t, chatID))
		if now == last {
			stableTicks++
			if stableTicks >= requiredTicks {
				return now
			}
			continue
		}
		stableTicks = 0
		last = now
	}
	t.Fatalf("waitSpyStable: telegram spy for chat %d kept growing past %s, final count=%d",
		chatID, stableFor, last)
	return last
}

// currentSpyForChat — one-shot read of telegram spy for chat_id.
func currentSpyForChat(t *testing.T, chatID int64) []testclient.TelegramSentMessage {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.GetTelegramSentWithResponse(context.Background(),
		&testclient.GetTelegramSentParams{ChatId: chatID})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Messages
}
