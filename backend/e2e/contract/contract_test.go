// Package contract — E2E тесты outbox-worker'а TrustMe (chunk 16).
//
// TestContractSending проходит chunk 16 outbox-flow от лица креатора:
// заявка переводится в `agreed` через POST /tma/campaigns/{secret_token}/agree
// со подписанным initData, затем мы синхронно дёргаем
// ContractSenderService.RunOnce через /test/trustme/run-outbox-once вместо
// ожидания @every 10s крон-тика. Все вызовы TrustMe идут в SpyOnlyClient
// (TRUSTME_MOCK=true в тестовой среде); записи доступны через
// /test/trustme/spy-list. Endpoint gated EnableTestEndpoints (404 в проде),
// поэтому spy возвращает сырые FIO/IIN/Phone — фикстуры синтетические.
// На happy-path тест ожидает один SendToSign-вызов с NumberDial формата
// UGC-{n}, audit-row campaign_creator.contract_initiated с actor_id=NULL и
// entity_id=cc.ID, ненулевой PDFSha256. На fail-and-recovery (spy-fail-next
// синтетически фейлит первую попытку) — два SendToSign'а с идентичным
// PDFSha256 (resend без re-render). Сценарии «empty template» и «known
// orphan finalize» закрыты repo/unit-уровнем — здесь t.Skip с явной ссылкой.
//
// Setup для каждого сценария — testutil.SetupCampaignWithInvitedCreator
// (готовая «приглашённая» пара) с уникальным suffix в email/handle, чтобы
// тесты были изолированы под t.Parallel(). Cleanup — defer-LIFO через
// E2E_CLEANUP=true.
package contract

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// tmaPostAgree — POST /tma/campaigns/{secret_token}/agree с авторизацией tma.
func tmaPostAgree(t *testing.T, secretToken, initData string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		testutil.BaseURL+"/tma/campaigns/"+secretToken+"/agree", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "tma "+initData)
	resp, err := testutil.HTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// runOutboxOnce триггерит контракт-worker через test-API.
func runOutboxOnce(t *testing.T) {
	t.Helper()
	tc := testutil.NewTestClient(t)
	resp, err := tc.TrustMeRunOutboxOnceWithResponse(context.Background())
	require.NoError(t, err)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode(),
		"runOutboxOnce: %d %s", resp.StatusCode(), string(resp.Body))
}

func spyList(t *testing.T) []testclient.TrustMeSentRecord {
	t.Helper()
	tc := testutil.NewTestClient(t)
	resp, err := tc.TrustMeSpyListWithResponse(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Items
}

// spyFail регистрирует постоянный синтетический сбой SendToSign на IIN —
// каждый attempt будет падать с reason до явного spyClearFail. Аналог
// telegramSpyFailNext (chat-keyed), но для TrustMe SpyOnlyClient. Устойчив
// к параллельным staging-worker тикам: pattern теста — register fail →
// дёрнуть worker один или несколько раз → проверить, что ВСЕ записи по
// нашему IIN зафейлены → clear fail → дёрнуть worker → success.
func spyFail(t *testing.T, iin string) {
	t.Helper()
	tc := testutil.NewTestClient(t)
	resp, err := tc.TrustMeSpyFailWithResponse(context.Background(),
		testclient.TrustMeSpyFailRequest{Iin: iin})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode())
}

// spyClearFail снимает fail-регистрацию для IIN, чтобы следующий
// SendToSign на этот IIN прошёл успешно.
func spyClearFail(t *testing.T, iin string) {
	t.Helper()
	tc := testutil.NewTestClient(t)
	resp, err := tc.TrustMeSpyClearFailWithResponse(context.Background(),
		testclient.TrustMeSpyClearFailRequest{Iin: iin})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode())
}

// spyMatchingByIIN читает spy-list и фильтрует по нашему IIN. Возвращает
// все записи в порядке вставки — каждый outbox-тик добавляет ровно одну
// запись на наш ряд, но параллельные worker-инстансы могут добавлять
// дополнительные между нашими шагами; тест должен ассертить инварианты
// (все pre-clear записи — с Err, post-clear — хотя бы одна без Err),
// а не точное количество.
func spyMatchingByIIN(t *testing.T, iin string) []testclient.TrustMeSentRecord {
	t.Helper()
	var ours []testclient.TrustMeSentRecord
	for _, r := range spyList(t) {
		if r.Iin == iin {
			ours = append(ours, r)
		}
	}
	return ours
}

// successfulAttempt returns the first record without Err (i.e. SendToSign
// completed). Used to assert post-clear-fail recovery converged.
func successfulAttempt(records []testclient.TrustMeSentRecord) (testclient.TrustMeSentRecord, bool) {
	for _, r := range records {
		if r.Err == nil || *r.Err == "" {
			return r, true
		}
	}
	return testclient.TrustMeSentRecord{}, false
}

// pdfShaIsHex64 — sanity-чек: spy положил полный hex sha256 (64 chars).
func pdfShaIsHex64(t *testing.T, sha string) {
	t.Helper()
	require.Len(t, sha, 64)
	for _, r := range sha {
		require.Truef(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'),
			"PdfSha256 must be lowercase hex, got %q", sha)
	}
}

// TestContractSending не использует t.Parallel() ни на корне, ни внутри:
// happy и recovery тики делят один SpyOnlyClient (process-wide) — параллельный
// запуск приводит к race-консьюму wildcard spy-fail-next и пропускам Phase 1
// claim'ов между параллельными тиками. Sequential — single-process гарантия
// того, что никто не «съедает» наш fail-next и не подхватывает наш agreed
// раньше нашего runOutboxOnce.
func TestContractSending(t *testing.T) {
	t.Run("happy path agreed → outbox runs → signing + spy record", func(t *testing.T) {
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		status, body := tmaPostAgree(t, fx.SecretToken, initData)
		require.Equalf(t, http.StatusOK, status, "agree body=%s", string(body))

		runOutboxOnce(t)

		// Изолируем нашу запись по уникальному IIN — testutil.UniqueIIN()
		// гарантирует уникальность между параллельными тестами.
		records := spyList(t)
		var matching []testclient.TrustMeSentRecord
		for _, r := range records {
			if r.Iin == fx.CreatorIIN {
				matching = append(matching, r)
			}
		}
		// Register cleanup BEFORE require.Len so a Fatalf cannot leak the
		// contracts row that Phase 1 already INSERT'ed. AdditionalInfo carries
		// the internal contracts.id even on failed SendToSign attempts.
		for _, r := range matching {
			testutil.RegisterContractCleanup(t, r.AdditionalInfo)
		}
		require.Len(t, matching, 1, "expected exactly one TrustMe record for our IIN")

		ours := matching[0]
		require.NotNil(t, ours.DocumentId)
		require.NotEmpty(t, *ours.DocumentId)
		require.Equal(t, fx.CreatorFIO, ours.Fio)
		require.Equal(t, fx.CreatorIIN, ours.Iin)
		require.Equal(t, fx.CreatorPhone, ours.Phone)
		require.Regexp(t, `^UGC-\d+$`, ours.NumberDial,
			"NumberDial должен иметь формат UGC-{serial}")
		require.Nil(t, ours.Err, "happy path must not record err")
		pdfShaIsHex64(t, ours.PdfSha256)

		// Audit-row campaign_creator.contract_initiated с actor_id=NULL и
		// entity_id=cc.ID.
		entry := testutil.FindAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator.contract_initiated")
		require.Nil(t, entry.ActorId)

		// cc.status flipped to signing после Phase 3 finalize. Енам в
		// openapi.yaml пока без `signing` (chunk 18 territory) — поэтому
		// сравниваем со string literal.
		listResp, err := fx.AdminClient.ListCampaignCreatorsWithResponse(context.Background(),
			uuid.MustParse(fx.CampaignID), testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		var ccItem *apiclient.CampaignCreator
		for i := range listResp.JSON200.Data.Items {
			if listResp.JSON200.Data.Items[i].Id.String() == fx.CampaignCreatorID {
				ccItem = &listResp.JSON200.Data.Items[i]
				break
			}
		}
		require.NotNil(t, ccItem, "campaign_creator must be in list")
		require.Equal(t, apiclient.CampaignCreatorStatus("signing"), ccItem.Status,
			"cc status must flip from agreed → signing after worker tick")
	})

	t.Run("empty template → not picked up", func(t *testing.T) {
		t.Skip("scenario unreachable through UI flow — repo-level test verifies SQL filter")
	})

	t.Run("soft-deleted campaign tombstone", func(t *testing.T) {
		t.Skip("scenario unreachable through public API — repo-level test verifies SQL filter")
	})

	t.Run("send fail → orphan recovered with same PDF (sha256 invariant)", func(t *testing.T) {
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		// Sticky fail на наш IIN ДО agree: каждый SendToSign на этот IIN
		// будет падать до явного spyClearFail. Под параллельным staging-
		// worker pool count-based one-shot был flaky — Phase 0 другого тика
		// мог consume единственную регистрацию между Phase 1 INSERT и нашим
		// проверкой spy.
		spyFail(t, fx.CreatorIIN)
		t.Cleanup(func() { spyClearFail(t, fx.CreatorIIN) })

		status, _ := tmaPostAgree(t, fx.SecretToken, initData)
		require.Equal(t, http.StatusOK, status)

		// Phase 1 создаёт contract_id, Phase 2b persist'ит PDF, Phase 2c
		// вызывает SendToSign — spy фейлит. Параллельные worker-инстансы
		// могут добавить ещё попытки Phase 0 recovery на тот же orphan;
		// все они должны fail, пока sticky активен. Дёргаем worker'а один
		// раз синхронно — после этого хотя бы одна fail-запись гарантирована.
		runOutboxOnce(t)

		afterFail := spyMatchingByIIN(t, fx.CreatorIIN)
		// Register cleanup BEFORE assertions — Phase 1 already INSERT'ed the
		// contracts row even though SendToSign failed. spy.AdditionalInfo
		// carries the internal contracts.id regardless of TrustMe outcome;
		// any retry on the same row reuses it, so registering for every
		// distinct AdditionalInfo here is safe and de-duplicating.
		registeredContracts := make(map[string]struct{})
		for _, r := range afterFail {
			if _, seen := registeredContracts[r.AdditionalInfo]; seen {
				continue
			}
			registeredContracts[r.AdditionalInfo] = struct{}{}
			testutil.RegisterContractCleanup(t, r.AdditionalInfo)
		}

		require.NotEmpty(t, afterFail,
			"after Tick #1 spy must record at least one SendToSign attempt on our IIN")
		for i, r := range afterFail {
			require.NotNilf(t, r.Err, "attempt %d must record err while sticky fail is active", i)
			require.NotEmptyf(t, *r.Err, "attempt %d err must be non-empty", i)
			require.Truef(t, r.DocumentId == nil || *r.DocumentId == "",
				"attempt %d must not carry a document_id (SendToSign failed)", i)
		}
		// All failed attempts share the same contracts.id — Phase 1 INSERT'ит
		// один ряд, Phase 0 recovery его же re-pick'ает.
		firstFail := afterFail[0]
		for _, r := range afterFail[1:] {
			require.Equalf(t, firstFail.AdditionalInfo, r.AdditionalInfo,
				"all retry attempts must share the same contracts.id")
			require.Equalf(t, firstFail.PdfSha256, r.PdfSha256,
				"retry attempts must not re-render the PDF (Decision #10)")
			require.Equalf(t, firstFail.NumberDial, r.NumberDial,
				"NumberDial (UGC-{serial}) must equal across retries")
		}

		// Clear sticky fail и дёргаем worker'а — Phase 0 поднимает orphan
		// (search вернёт ErrTrustMeNotFound) → resend с persisted PDF.
		// Параллельные worker'ы тоже могут подобрать ряд после clear — нам
		// важен инвариант "хотя бы одна success запись с тем же contracts.id
		// и тем же PdfSha256", а не точный count.
		spyClearFail(t, fx.CreatorIIN)
		runOutboxOnce(t)

		afterRecovery := spyMatchingByIIN(t, fx.CreatorIIN)
		require.Greater(t, len(afterRecovery), len(afterFail),
			"Tick after clear must add at least one new SendToSign attempt")

		success, ok := successfulAttempt(afterRecovery)
		require.True(t, ok, "after clear, at least one attempt on our IIN must succeed")
		require.NotNil(t, success.DocumentId)
		require.NotEmpty(t, *success.DocumentId)

		// Same contracts.id, same PDF — recovery resend без re-render.
		require.Equal(t, firstFail.AdditionalInfo, success.AdditionalInfo,
			"recovery success must reuse the same contracts.id as the failed attempts")
		require.Equal(t, firstFail.PdfSha256, success.PdfSha256,
			"recovery success must reuse the persisted PDF (Decision #10)")
		require.Equal(t, firstFail.NumberDial, success.NumberDial,
			"NumberDial (UGC-{serial}) must equal across fail and recovery")
	})

	t.Run("known orphan finalize without re-send", func(t *testing.T) {
		t.Skip("covered by unit TestContractSenderService_Phase0_KnownDoc_Finalize")
	})
}
