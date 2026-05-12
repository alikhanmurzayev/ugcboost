// Package tma — E2E тесты HTTP-поверхности /tma/*.
//
// TestTmaDecisionFlow проходит полный happy-path жизненного цикла решения
// креатора: с заранее invited campaign_creator-row (через
// SetupCampaignWithInvitedCreator), TMA-клиент подписывает initData через
// /test/tma/sign-init-data и шлёт POST /tma/campaigns/{secret_token}/agree.
// Сценарии внутри: первый клик — 200 + already_decided=false и
// status=agreed; повтор того же клика — 200 + already_decided=true без
// нового audit-row и без UPDATE; попытка decline из agreed — 422
// CAMPAIGN_CREATOR_ALREADY_AGREED.
//
// TestTmaDecisionDeclineFlow зеркалит agree-flow для decline: invited →
// declined; повтор — 200 + already_decided=true; agree из declined — 422
// CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE.
//
// TestTmaDecisionFromPlanned проверяет 422 CAMPAIGN_CREATOR_NOT_INVITED
// для row в статусе planned (creator добавлен через A1, но без A4 notify).
//
// TestTmaDecisionUnauthorized проходит шесть вариантов 401: отсутствующий
// заголовок, неверный scheme (Bearer вместо tma), пустой hash, garbage
// initData, истёкший auth_date, auth_date в будущем и HMAC mismatch
// (initData подписан другим bot_token). Все возвращают идентичный generic
// body с CodeUnauthorized — middleware не разглашает причину провала, чтобы
// атакующий не мог отличить «битый формат» от «неверная подпись».
//
// TestTmaDecisionRegexReject404 проверяет ранний reject в handler: путь с
// secret_token короче 16 символов получает 404 CAMPAIGN_NOT_FOUND до любого
// обращения к БД (initData всё равно валидируется middleware'ом сначала, но
// regex срабатывает до AuthzService.GetBySecretToken).
//
// TestTmaDecisionForbidden и TestTmaDecisionCampaignNotFound покрывают
// 403 (creator не в кампании) и 404 (regex-valid но несуществующий
// secret_token) ветки AuthzService. Для 403 — creator зарегистрирован в
// одной кампании, но инициирует решение для secret_token другой.
//
// TestTmaDecisionAntiFingerprint сравнивает три ветки попарно: «creator не
// зарегистрирован» (TG-user без creator-row), «creator зарегистрирован, но
// не в этой кампании» и «secret_token не существует». Все три обязаны
// возвращать одинаковый response body — иначе атакующий по разнице ответов
// узнает, существует ли creator-row под его TG-id или есть ли invited link.
// Soft-deleted кампания формально даёт ту же 404-ветку, но e2e не имеет
// бизнес/тест-ручки soft-delete'а — покрытие is_deleted-фильтра живёт на
// repo-уровне (`TestCampaignRepository_GetBySecretToken`).
//
// TestTmaGetParticipation покрывает read-only GET
// /tma/campaigns/{secret_token}/participation, который TMA дёргает на mount
// страницы ТЗ, чтобы решить, рендерить ли кнопки accept/decline. Эндпоинт
// разделяет regex/authz pre-pass с agree/decline (TestTmaDecision*
// покрывают эти ветки), поэтому здесь — один happy-path: invited creator
// получает 200 + status="invited" из своей кампании.
//
// Setup для каждого теста независимо собирает кампанию + invited creator
// через testutil.SetupCampaignWithInvitedCreator; cleanup идёт через
// /test/cleanup-entity при E2E_CLEANUP=true.
package tma

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

const (
	tmaPathPrefix = "/tma/campaigns/"
)

// tmaPost is a thin wrapper around testutil.NewAPIClient — strict-server
// `tmaInitData` security scheme is not modeled in apiclient (the security
// scheme generates a `tmaInitData` argument we have no helper for); raw
// HTTP keeps the test code mirroring the real Telegram WebApp surface.
func tmaPost(t *testing.T, secretToken, action, initData string) (status int, body []byte) {
	t.Helper()
	return tmaPostWithScheme(t, secretToken, action, "tma", initData)
}

// tmaPostWithScheme lets negative tests pick a non-`tma` Authorization scheme
// (or a malformed initData literal) — the middleware rejection path is the
// same regardless of why the header is invalid.
func tmaPostWithScheme(t *testing.T, secretToken, action, scheme, payload string) (status int, body []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		testutil.BaseURL+tmaPathPrefix+secretToken+"/"+action, nil)
	require.NoError(t, err)
	if scheme != "" || payload != "" {
		header := scheme
		if scheme != "" && payload != "" {
			header += " "
		}
		header += payload
		req.Header.Set("Authorization", header)
	}
	resp, err := testutil.HTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// tmaGetParticipation is the read-only GET counterpart to tmaPost. The
// /participation endpoint shares the secretToken / initData contract.
func tmaGetParticipation(t *testing.T, secretToken, initData string) (status int, body []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		testutil.BaseURL+tmaPathPrefix+secretToken+"/participation", nil)
	require.NoError(t, err)
	if initData != "" {
		req.Header.Set("Authorization", "tma "+initData)
	}
	resp, err := testutil.HTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// decodeParticipation decodes the response body as the typed apiclient
// TmaParticipationResult.
func decodeParticipation(t *testing.T, body []byte) apiclient.TmaParticipationResult {
	t.Helper()
	var got apiclient.TmaParticipationResult
	require.NoErrorf(t, json.Unmarshal(body, &got), "decode TmaParticipationResult: %s", string(body))
	return got
}

// decodeDecision decodes the response body as the typed apiclient
// TmaDecisionResult. Body-string `Contains` was an anti-pattern: a schema
// drift (e.g. `status` migrating into `data.status`) would still match
// substring-wise. This helper makes the contract explicit.
func decodeDecision(t *testing.T, body []byte) apiclient.TmaDecisionResult {
	t.Helper()
	var got apiclient.TmaDecisionResult
	require.NoErrorf(t, json.Unmarshal(body, &got), "decode TmaDecisionResult: %s", string(body))
	return got
}

// decodeError decodes the response body as the typed apiclient ErrorResponse.
func decodeError(t *testing.T, body []byte) apiclient.ErrorResponse {
	t.Helper()
	var got apiclient.ErrorResponse
	require.NoErrorf(t, json.Unmarshal(body, &got), "decode ErrorResponse: %s", string(body))
	return got
}

func TestTmaDecisionFlow(t *testing.T) {
	t.Parallel()

	t.Run("happy agree → 200 already_decided=false then idempotent", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		status, body := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equalf(t, http.StatusOK, status, "first agree body=%s", string(body))
		first := decodeDecision(t, body)
		require.Equal(t, apiclient.CampaignCreatorStatus(apiclient.Agreed), first.Status)
		require.False(t, first.AlreadyDecided)

		// Audit row: action=campaign_creator_agree, actor_id=NULL (TMA flow
		// has no admin actor), payload carries (campaign_id, creator_id).
		entry := testutil.FindAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator_agree")
		require.Nil(t, entry.ActorId, "TMA decision audit row must have actor_id=NULL")
		require.NotNil(t, entry.NewValue)

		// Idempotent repeat — must NOT add a second audit row.
		// We deliberately do NOT assert repeat.Status here: between the two
		// POST requests the outbox worker may pick up the freshly-agreed row,
		// hand the contract off to TrustMe, and flip status from "agreed" to
		// "signing". Both outcomes satisfy the idempotency contract — the
		// invariants we actually care about are (a) AlreadyDecided=true on
		// the repeat call, and (b) only one campaign_creator_agree audit row
		// gets written. Pinning a specific transient status would re-introduce
		// a flake we already had to fix once with TrustMe SpyOnly mocking.
		status, body = tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equalf(t, http.StatusOK, status, "repeat agree body=%s", string(body))
		repeat := decodeDecision(t, body)
		require.True(t, repeat.AlreadyDecided)

		entries := testutil.ListAuditEntriesByAction(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator_agree")
		require.Len(t, entries, 1, "idempotent agree must not write a second audit row")
	})

	t.Run("decline from agreed → 422 CAMPAIGN_CREATOR_ALREADY_AGREED", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		_, _ = tmaPost(t, fx.SecretToken, "agree", initData)
		status, body := tmaPost(t, fx.SecretToken, "decline", initData)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		errResp := decodeError(t, body)
		require.Equal(t, "CAMPAIGN_CREATOR_ALREADY_AGREED", errResp.Error.Code)
	})
}

func TestTmaDecisionDeclineFlow(t *testing.T) {
	t.Parallel()

	t.Run("happy decline → 200 then idempotent", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		status, body := tmaPost(t, fx.SecretToken, "decline", initData)
		require.Equalf(t, http.StatusOK, status, "first decline body=%s", string(body))
		first := decodeDecision(t, body)
		require.Equal(t, apiclient.CampaignCreatorStatus(apiclient.Declined), first.Status)
		require.False(t, first.AlreadyDecided)

		status, body = tmaPost(t, fx.SecretToken, "decline", initData)
		require.Equalf(t, http.StatusOK, status, "repeat decline body=%s", string(body))
		repeat := decodeDecision(t, body)
		require.Equal(t, apiclient.CampaignCreatorStatus(apiclient.Declined), repeat.Status)
		require.True(t, repeat.AlreadyDecided)
	})

	t.Run("agree from declined → 422 CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		_, _ = tmaPost(t, fx.SecretToken, "decline", initData)
		status, body := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		errResp := decodeError(t, body)
		require.Equal(t, "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE", errResp.Error.Code)
	})
}

func TestTmaDecisionUnauthorized(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)

	t.Run("missing header → 401 generic", func(t *testing.T) {
		t.Parallel()
		status, body := tmaPostWithScheme(t, fx.SecretToken, "agree", "", "")
		require.Equal(t, http.StatusUnauthorized, status)
		errResp := decodeError(t, body)
		require.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
	})

	t.Run("wrong scheme → 401 generic", func(t *testing.T) {
		t.Parallel()
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})
		status, body := tmaPostWithScheme(t, fx.SecretToken, "agree", "Bearer", initData)
		require.Equal(t, http.StatusUnauthorized, status)
		errResp := decodeError(t, body)
		require.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
	})

	t.Run("garbage initData → 401", func(t *testing.T) {
		t.Parallel()
		status, _ := tmaPost(t, fx.SecretToken, "agree", "garbage_payload")
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("empty hash → 401", func(t *testing.T) {
		t.Parallel()
		// Valid auth_date and minimal user payload but the `hash=` field
		// is empty — middleware must reject without leaking the reason.
		header := "auth_date=" + time.Now().Format("20060102") + `&user={"id":1}&hash=`
		status, _ := tmaPost(t, fx.SecretToken, "agree", header)
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("expired auth_date → 401", func(t *testing.T) {
		t.Parallel()
		past := time.Now().Add(-48 * time.Hour).Unix()
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{AuthDate: &past})
		status, _ := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("auth_date in future → 401", func(t *testing.T) {
		t.Parallel()
		future := time.Now().Add(2 * time.Hour).Unix()
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{AuthDate: &future})
		status, _ := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("HMAC mismatch (last char of hash flipped) → 401", func(t *testing.T) {
		t.Parallel()
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})
		// SignInitData returns a query string ending with `&hash=<hex>`.
		// Flip the last char so the hex remains well-formed but the hash
		// no longer matches — middleware must reach the constant-time
		// compare and reject.
		mutated := flipLastByte(initData)
		require.NotEqual(t, initData, mutated, "test setup must actually mutate the hash")
		status, body := tmaPost(t, fx.SecretToken, "agree", mutated)
		require.Equal(t, http.StatusUnauthorized, status)
		errResp := decodeError(t, body)
		require.Equal(t, "UNAUTHORIZED", errResp.Error.Code)
	})
}

func TestTmaDecisionRegexReject404(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)
	initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

	t.Run("short token → 404 without DB lookup", func(t *testing.T) {
		t.Parallel()
		status, body := tmaPost(t, "short", "agree", initData)
		require.Equal(t, http.StatusNotFound, status)
		errResp := decodeError(t, body)
		require.Equal(t, "CAMPAIGN_NOT_FOUND", errResp.Error.Code)
	})
}

func TestTmaDecisionForbidden(t *testing.T) {
	t.Parallel()
	// The creator is invited into one campaign; we attempt the decision on a
	// completely unrelated campaign's secret_token — anti-fingerprint 403.
	fxA := testutil.SetupCampaignWithInvitedCreator(t)
	fxB := testutil.SetupCampaignWithInvitedCreator(t)
	initData := testutil.SignInitData(t, fxA.TelegramUserID, testutil.SignInitDataOpts{})

	status, body := tmaPost(t, fxB.SecretToken, "agree", initData)
	require.Equal(t, http.StatusForbidden, status)
	errResp := decodeError(t, body)
	require.Equal(t, "TMA_FORBIDDEN", errResp.Error.Code)
}

func TestTmaDecisionCampaignNotFound(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)
	initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

	// Use a regex-valid but non-existent secret_token — handler regex-passes,
	// AuthzService.GetBySecretToken hits sql.ErrNoRows → 404.
	bogusToken := "bogus_unknown_secrettokenxx"
	status, body := tmaPost(t, bogusToken, "agree", initData)
	require.Equal(t, http.StatusNotFound, status)
	errResp := decodeError(t, body)
	require.Equal(t, "CAMPAIGN_NOT_FOUND", errResp.Error.Code)
}

func TestTmaDecisionAntiFingerprint(t *testing.T) {
	t.Parallel()
	// Three branches that, ideologically, must respond identically:
	//
	//   * not-registered creator: TG-user without a creators-row →
	//     middleware passes (telegram_user_id only), AuthzService finds
	//     role missing in ctx → ErrTMAForbidden.
	//   * creator-not-in-this-campaign: TG-user has a creator-row but
	//     belongs to another campaign → ErrTMAForbidden.
	//   * unknown-secret-token: regex-valid token that never existed →
	//     ErrCampaignNotFound (different status/code by design).
	//
	// The first two share status+code+message; the third returns 404 by
	// design. Asserting the first two are byte-identical and the third
	// has a stable distinct shape locks the anti-fingerprint contract:
	// any future drift between not-registered vs not-in-campaign would
	// leak to an attacker which TG-id is already a creator on the
	// platform.
	fxOwn := testutil.SetupCampaignWithInvitedCreator(t)
	fxOther := testutil.SetupCampaignWithInvitedCreator(t)

	// Synthetic Telegram user-id that does not exist as a creator row.
	notRegisteredTGID := int64(900_000_0099) + time.Now().UnixNano()%1_000
	initDataAnon := testutil.SignInitData(t, notRegisteredTGID, testutil.SignInitDataOpts{})
	statusAnon, bodyAnon := tmaPost(t, fxOwn.SecretToken, "agree", initDataAnon)

	// Creator who is registered but invited only into fxOther.
	initDataOther := testutil.SignInitData(t, fxOther.TelegramUserID, testutil.SignInitDataOpts{})
	statusOtherCC, bodyOther := tmaPost(t, fxOwn.SecretToken, "agree", initDataOther)

	require.Equal(t, statusAnon, statusOtherCC,
		"not-registered and not-in-campaign must share HTTP status (anti-fingerprint)")
	require.Equal(t, http.StatusForbidden, statusAnon)

	errAnon := decodeError(t, bodyAnon)
	errOther := decodeError(t, bodyOther)
	require.Equal(t, errAnon, errOther,
		"not-registered vs not-in-campaign error bodies must be byte-equal")
	require.Equal(t, "TMA_FORBIDDEN", errAnon.Error.Code)

	// The unknown-secret-token branch is documented as 404 — separate from
	// the 403 fingerprint family. Asserted to keep the contract explicit.
	statusBogus, bodyBogus := tmaPost(t,
		"bogus_unknown_secrettokenxx", "agree", initDataOther)
	require.Equal(t, http.StatusNotFound, statusBogus)
	errBogus := decodeError(t, bodyBogus)
	require.Equal(t, "CAMPAIGN_NOT_FOUND", errBogus.Error.Code)
}

func TestTmaDecisionFromPlanned(t *testing.T) {
	t.Parallel()
	// Build a `planned` campaign_creator row by calling A1 (Add) without A4
	// (Notify). The TMA decision endpoints must surface 422
	// CAMPAIGN_CREATOR_NOT_INVITED until the admin sends an invitation.
	c, adminToken, _ := testutil.SetupAdminClient(t)
	uniq := testutil.UniqueEmail("plannedtma")
	tmaToken := "planned_" + uniq[5:25]
	for len(tmaToken) < 16 {
		tmaToken += "x"
	}
	tmaURL := "https://tma.ugcboost.kz/tz/" + tmaToken
	createResp, err := c.CreateCampaignWithResponse(context.Background(),
		apiclient.CreateCampaignJSONRequestBody{Name: "Promo-" + uniq, TmaUrl: tmaURL},
		testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode())
	campaignID := createResp.JSON201.Data.Id.String()
	testutil.RegisterCampaignCleanup(t, campaignID)

	suffix := testutil.UniqueIIN()[6:]
	creator := testutil.SetupApprovedCreator(t, testutil.CreatorApplicationFixture{
		Socials: []testutil.SocialFixture{
			{Platform: "instagram", Handle: "plannedtma_ig_" + suffix, Verification: testutil.VerificationAutoIG},
			{Platform: "tiktok", Handle: "plannedtma_tt_" + suffix, Verification: testutil.VerificationNone},
		},
	})
	creatorUUID := uuid.MustParse(creator.CreatorID)
	addResp, err := c.AddCampaignCreatorsWithResponse(context.Background(), uuid.MustParse(campaignID),
		apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
		testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, addResp.StatusCode())
	testutil.RegisterCampaignCreatorCleanup(t, c, adminToken, campaignID, creator.CreatorID)

	initData := testutil.SignInitData(t, creator.TelegramUserID, testutil.SignInitDataOpts{})

	status, body := tmaPost(t, tmaToken, "agree", initData)
	require.Equal(t, http.StatusUnprocessableEntity, status)
	errResp := decodeError(t, body)
	require.Equal(t, "CAMPAIGN_CREATOR_NOT_INVITED", errResp.Error.Code)
}

func TestTmaGetParticipation(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)
	initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

	status, body := tmaGetParticipation(t, fx.SecretToken, initData)
	require.Equalf(t, http.StatusOK, status, "body=%s", string(body))
	got := decodeParticipation(t, body)
	require.Equal(t, apiclient.CampaignCreatorStatus(apiclient.Invited), got.Status)
}

// flipLastByte mutates the final character of s — for crafting a hash that
// is structurally valid hex but does not match the computed HMAC. Used by
// the HMAC-mismatch negative case.
func flipLastByte(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	last := b[len(b)-1]
	switch {
	case last == '0':
		last = '1'
	case last >= '1' && last <= '9':
		last--
	case last == 'a':
		last = 'b'
	case last >= 'b' && last <= 'f':
		last--
	case last == 'A':
		last = 'B'
	case last >= 'B' && last <= 'F':
		last--
	default:
		last = '0'
	}
	b[len(b)-1] = last
	return string(b)
}
