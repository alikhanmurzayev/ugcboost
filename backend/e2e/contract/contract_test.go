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

func spyFailNext(t *testing.T, additionalInfo string, count int) {
	t.Helper()
	tc := testutil.NewTestClient(t)
	body := testclient.TrustMeSpyFailNextRequest{
		AdditionalInfo: additionalInfo,
		Count:          &count,
	}
	resp, err := tc.TrustMeSpyFailNextWithResponse(context.Background(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode())
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

		status, _ := tmaPostAgree(t, fx.SecretToken, initData)
		require.Equal(t, http.StatusOK, status)

		// Wildcard fail-next: ровно одну следующую отправку SpyClient зафейлит
		// независимо от additionalInfo (contract_id ещё не существует на момент
		// регистрации). Используется в Phase 0 recovery e2e сценарии.
		spyFailNext(t, "", 1)

		// Tick #1: Phase 1 создаёт contract_id, Phase 2b persist'ит PDF, Phase
		// 2c фейлит. Ряд остаётся orphan'ом.
		runOutboxOnce(t)

		// Tick #2: Phase 0 поднимает orphan → search вернёт ErrTrustMeNotFound
		// → resend с persisted PDF. Sha256 должен совпасть.
		runOutboxOnce(t)

		records := spyList(t)
		var ours []testclient.TrustMeSentRecord
		for _, r := range records {
			if r.Iin == fx.CreatorIIN {
				ours = append(ours, r)
			}
		}
		require.Len(t, ours, 2, "recovery requires exactly fail + success attempts on our IIN")

		// Первая попытка должна быть с error (spyFailNext), вторая успешная.
		require.NotNil(t, ours[0].Err)
		require.NotEmpty(t, *ours[0].Err)
		require.Nil(t, ours[1].Err, "second attempt (recovery) must succeed")
		require.NotNil(t, ours[1].DocumentId)
		require.NotEmpty(t, *ours[1].DocumentId)

		// Sha256 PDF идентичный — Phase 0 resend без re-render (Decision #10).
		require.Equal(t, ours[0].PdfSha256, ours[1].PdfSha256,
			"PdfSha256 must equal between fail and recovery (no re-render)")
		require.Equal(t, ours[0].AdditionalInfo, ours[1].AdditionalInfo,
			"additionalInfo (=contract.id) must equal between attempts")
		require.Equal(t, ours[0].NumberDial, ours[1].NumberDial,
			"NumberDial (UGC-{serial}) одинаковый — serial_number один на ряд")
	})

	t.Run("known orphan finalize without re-send", func(t *testing.T) {
		t.Skip("covered by unit TestContractSenderService_Phase0_KnownDoc_Finalize")
	})
}
