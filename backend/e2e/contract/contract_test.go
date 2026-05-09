// Package contract — E2E тесты outbox-worker'а TrustMe (chunk 16).
//
// TestContractSending проходит chunk 16 outbox-flow от лица креатора:
// заявка переводится в `agreed` через POST /tma/campaigns/{secret_token}/agree
// со подписанным initData, затем мы синхронно дёргаем
// ContractSenderService.RunOnce через /test/trustme/run-outbox-once вместо
// ожидания @every 10s крон-тика. Все вызовы TrustMe идут в SpyOnlyClient
// (TRUSTME_MOCK=true в тестовой среде), записи доступны через
// /test/trustme/spy-list — оттуда читаем sha256-фингерпринт PDF и
// фингерпринты PII-полей (raw FIO/IIN/Phone и raw PDF в response не
// эспонируются: security.md hard rule). На happy-path тест ожидает один
// SendToSign-вызов, audit-row campaign_creator.contract_initiated с
// actor_id=NULL и entity_id=cc.ID, и сохранённый ненулевой PDFSha256.
// На fail-and-recovery (spy-fail-next ставит синтетический сбой первой
// попытки) — два SendToSign-вызова с идентичным PDFSha256 (без re-render
// на recovery). Скоупы «empty template» и «known orphan finalize» закрыты
// repo/unit-уровнем — здесь лишь t.Skip с явной ссылкой на покрывающий
// тест.
//
// Setup для каждого сценария — testutil.SetupCampaignWithInvitedCreator
// (готовая «приглашённая» пара) с уникальным suffix в email/handle,
// чтобы тесты были изолированы под `t.Parallel()`. Cleanup — defer-LIFO
// через E2E_CLEANUP=true.
package contract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// fingerprint16 — реплика `internal/trustme/spy.Fingerprint`: первые 8 байт
// sha256(value), hex (16 chars). E2e module не может импортировать internal,
// поэтому копия. Должна совпадать с production алгоритмом, иначе фильтрация
// spy-list по IIN/FIO/Phone не будет находить записи.
func fingerprint16(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

// tmaPostAgree — тонкая обёртка над POST /tma/campaigns/{secret_token}/agree.
// Отдельная функция, чтобы каждый t.Run не повторял boilerplate.
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

// runOutboxOnce триггерит контракт-worker через test-API. Возвращает 204
// или фейлит тест.
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

// pdfShaIsHex64 — sanity-чек: spy положил полный hex sha256 (64 chars). PDF
// сам не экспонируется (PII в overlay'е), поэтому parseability проверяет
// unit-тест RealRenderer, а e2e — только что worker действительно посчитал
// sha256 от non-empty PDF и положил в response.
func pdfShaIsHex64(t *testing.T, sha string) {
	t.Helper()
	require.Len(t, sha, 64)
	for _, r := range sha {
		require.Truef(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'),
			"PdfSha256 must be lowercase hex, got %q", sha)
	}
}

func TestContractSending(t *testing.T) {
	t.Parallel()

	t.Run("happy path agreed → outbox runs → signing + spy record", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		status, body := tmaPostAgree(t, fx.SecretToken, initData)
		require.Equalf(t, http.StatusOK, status, "agree body=%s", string(body))

		runOutboxOnce(t)

		// Filter spy-list строго по IINFingerprint от уникального IIN этого
		// теста — UniqueIIN() гарантирует уникальность между параллельными
		// тестами, поэтому только наша запись попадёт в matching.
		expectedIINFP := fingerprint16(fx.CreatorIIN)
		records := spyList(t)
		var matching []testclient.TrustMeSentRecord
		for _, r := range records {
			if r.IinFingerprint == expectedIINFP {
				matching = append(matching, r)
			}
		}
		require.Len(t, matching, 1, "expected exactly one TrustMe record for our IIN")

		ours := matching[0]
		require.NotNil(t, ours.DocumentId)
		require.NotEmpty(t, *ours.DocumentId)
		require.Equal(t, fingerprint16(fx.CreatorFIO), ours.FioFingerprint)
		require.Equal(t, expectedIINFP, ours.IinFingerprint)
		require.Equal(t, fingerprint16("+77071234567"), ours.PhoneFingerprint,
			"normalized phone fingerprint must match — ApprovedCreatorFixture phone always +77071234567")
		require.Nil(t, ours.Err, "happy path must not record err")
		pdfShaIsHex64(t, ours.PdfSha256)

		// Audit-row campaign_creator.contract_initiated с actor_id=NULL и
		// entity_id=cc.ID. Фильтр по cc.ID стал точным после fix B (раньше
		// ошибочно entity_id=contract.ID — read-side queries по cc не
		// находили строку).
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
		t.Parallel()
		// E2E-сценарий невозможен через UI flow: chunk 12 notify-guard
		// блокирует invite креатора на кампанию без contract_template_pdf
		// (CONTRACT_TEMPLATE_REQUIRED), значит status=`agreed` без шаблона
		// можно создать только direct-DB-mutation, что не предусмотрено
		// тест-API. Фильтр `length(c.contract_template_pdf) > 0` в Phase 1
		// SELECT покрыт через `TestContractRepository_SelectAgreedForClaim`
		// (SQL-strict-equal на pgxmock-уровне).
		t.Skip("scenario unreachable through UI flow — repo-level test verifies SQL filter")
	})

	t.Run("soft-deleted campaign tombstone", func(t *testing.T) {
		t.Parallel()
		// Soft-delete кампании отсутствует в публичном/тестовом API (только
		// hard-delete через /test/cleanup-entity, которое не выставляет
		// is_deleted=true). Фильтр `c.is_deleted = false` в Phase 1 SELECT
		// покрыт `TestContractRepository_SelectAgreedForClaim` через
		// strict-SQL pgxmock match.
		t.Skip("scenario unreachable through public API — repo-level test verifies SQL filter")
	})

	t.Run("send fail → orphan recovered with same PDF (sha256 invariant)", func(t *testing.T) {
		t.Parallel()
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
		// → resend с persisted PDF. Sha256 должен совпасть — это ключевой
		// invariant спеки line 116.
		runOutboxOnce(t)

		// Filter spy-list строго по IINFingerprint этого теста — точная
		// изоляция между параллельными e2e через UniqueIIN.
		expectedIINFP := fingerprint16(fx.CreatorIIN)
		records := spyList(t)
		var ours []testclient.TrustMeSentRecord
		for _, r := range records {
			if r.IinFingerprint == expectedIINFP {
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

		// Sha256 PDF идентичный — Phase 0 resend без re-render (intent
		// Decision #10 «PDF уже в БД»). Server уже посчитал sha256 от
		// исходного base64; e2e сравнивает напрямую.
		require.Equal(t, ours[0].PdfSha256, ours[1].PdfSha256,
			"PdfSha256 must equal between fail and recovery (no re-render)")
		require.Equal(t, ours[0].AdditionalInfo, ours[1].AdditionalInfo,
			"additionalInfo (=contract.id) must equal between attempts")
	})

	t.Run("known orphan finalize without re-send", func(t *testing.T) {
		t.Parallel()
		// Сценарий «TrustMe знает наш orphan» требует чтобы между Phase 1
		// commit и Phase 2c send в БД остался ряд с trustme_document_id=NULL.
		// Воспроизвести через test-API нельзя без direct-mutation contracts.
		// `TestContractSenderService_Phase0_KnownDoc_Finalize` unit-тест
		// проверяет ветвь полностью: search → finalize-without-resend +
		// сохранение search.ContractStatus + audit с cc_id.
		t.Skip("covered by unit TestContractSenderService_Phase0_KnownDoc_Finalize")
	})
}
