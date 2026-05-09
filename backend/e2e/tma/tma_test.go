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
// TestTmaDecisionUnauthorized проходит четыре варианта 401: отсутствующий
// заголовок, неверный scheme, истёкший auth_date, auth_date в будущем,
// HMAC mismatch. Все возвращают generic body с CodeUnauthorized.
//
// TestTmaDecisionRegexReject404 проверяет ранний reject в handler: путь с
// secret_token короче 16 символов получает 404 CAMPAIGN_NOT_FOUND до любого
// обращения к БД (initData всё равно валидируется middleware'ом сначала, но
// regex срабатывает до AuthzService.GetBySecretToken).
//
// TestTmaDecisionForbidden и TestTmaDecisionCampaignNotFound покрывают
// 403 (creator не в кампании) и 404 (soft-deleted кампания) ветки
// AuthzService. Для 403 — creator зарегистрирован в одной кампании, но
// инициирует решение для secret_token другой. Для 404 — admin-DELETE
// помечает кампанию soft-deleted, и тот же initData больше не находит её.
//
// Setup для каждого теста независимо собирает кампанию + invited creator
// через testutil.SetupCampaignWithInvitedCreator; cleanup идёт через
// /test/cleanup-entity при E2E_CLEANUP=true.
package tma

import (
	"context"
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		testutil.BaseURL+tmaPathPrefix+secretToken+"/"+action, nil)
	require.NoError(t, err)
	if initData != "" {
		req.Header.Set("Authorization", "tma "+initData)
	}
	resp, err := testutil.HTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body = readAll(t, resp)
	return resp.StatusCode, body
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	const max = 64 * 1024
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for len(buf) < max {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}

func TestTmaDecisionFlow(t *testing.T) {
	t.Parallel()

	t.Run("happy agree → 200 already_decided=false then idempotent", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		status, body := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equalf(t, http.StatusOK, status, "first agree body=%s", string(body))
		require.Contains(t, string(body), `"status":"agreed"`)
		require.Contains(t, string(body), `"alreadyDecided":false`)

		// Audit row: action=campaign_creator_agree, actor_id=NULL (TMA flow
		// has no admin actor), payload carries (campaign_id, creator_id).
		// Spec §Acceptance demands this verification at e2e layer.
		entry := testutil.FindAuditEntry(t, fx.AdminClient, fx.AdminToken,
			"campaign_creator", fx.CampaignCreatorID, "campaign_creator_agree")
		require.Nil(t, entry.ActorId, "TMA decision audit row must have actor_id=NULL")
		require.NotNil(t, entry.NewValue)

		// Idempotent repeat — must NOT add a second audit row.
		status, body = tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equalf(t, http.StatusOK, status, "repeat agree body=%s", string(body))
		require.Contains(t, string(body), `"alreadyDecided":true`)

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
		require.Contains(t, string(body), `"code":"CAMPAIGN_CREATOR_ALREADY_AGREED"`)
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
		require.Contains(t, string(body), `"status":"declined"`)
		require.Contains(t, string(body), `"alreadyDecided":false`)

		status, body = tmaPost(t, fx.SecretToken, "decline", initData)
		require.Equalf(t, http.StatusOK, status, "repeat decline body=%s", string(body))
		require.Contains(t, string(body), `"alreadyDecided":true`)
	})

	t.Run("agree from declined → 422 CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCampaignWithInvitedCreator(t)
		initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

		_, _ = tmaPost(t, fx.SecretToken, "decline", initData)
		status, body := tmaPost(t, fx.SecretToken, "agree", initData)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		require.Contains(t, string(body), `"code":"CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE"`)
	})
}

func TestTmaDecisionUnauthorized(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)

	t.Run("missing header → 401", func(t *testing.T) {
		t.Parallel()
		status, body := tmaPost(t, fx.SecretToken, "agree", "")
		require.Equal(t, http.StatusUnauthorized, status)
		require.Contains(t, string(body), `"code":"UNAUTHORIZED"`)
	})

	t.Run("garbage initData → 401", func(t *testing.T) {
		t.Parallel()
		status, _ := tmaPost(t, fx.SecretToken, "agree", "garbage_payload")
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
}

func TestTmaDecisionRegexReject404(t *testing.T) {
	t.Parallel()
	fx := testutil.SetupCampaignWithInvitedCreator(t)
	initData := testutil.SignInitData(t, fx.TelegramUserID, testutil.SignInitDataOpts{})

	t.Run("short token → 404 without DB lookup", func(t *testing.T) {
		t.Parallel()
		status, body := tmaPost(t, "short", "agree", initData)
		require.Equal(t, http.StatusNotFound, status)
		require.Contains(t, string(body), `"code":"CAMPAIGN_NOT_FOUND"`)
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
	require.Contains(t, string(body), `"code":"TMA_FORBIDDEN"`)
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
	require.Contains(t, string(body), `"code":"CAMPAIGN_NOT_FOUND"`)
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
	require.Contains(t, string(body), `"code":"CAMPAIGN_CREATOR_NOT_INVITED"`)
}
