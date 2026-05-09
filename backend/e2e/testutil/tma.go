package testutil

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// SignInitDataOpts mirrors the optional fields of testclient.SignTMAInitDataRequest
// — used by negative tests that need to forge an expired or future-dated
// signed payload.
type SignInitDataOpts struct {
	AuthDate *int64
}

// SignInitData calls POST /test/tma/sign-init-data and returns the signed
// initData query string. Test endpoint only — registered when
// EnableTestEndpoints=true (i.e. ENVIRONMENT != production).
func SignInitData(t *testing.T, telegramUserID int64, opts SignInitDataOpts) string {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.SignTMAInitDataWithResponse(context.Background(),
		testclient.SignTMAInitDataJSONRequestBody{
			TelegramUserId: telegramUserID,
			AuthDate:       opts.AuthDate,
		})
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode(),
		"SignInitData: unexpected status %d", resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.InitData
}

// TmaCampaignFixture bundles the persisted state of a TMA-flow setup —
// everything tma_test.go needs to drive agree/decline scenarios end-to-end.
type TmaCampaignFixture struct {
	CampaignID        string
	CreatorID         string
	CampaignCreatorID string
	SecretToken       string
	TmaURL            string
	TelegramUserID    int64
	AdminToken        string
	AdminClient       *apiclient.ClientWithResponses
}

// SetupCampaignWithInvitedCreator builds the full happy-path setup for the
// TMA decision flow:
//
//  1. Admin client + a fresh campaign with a valid tma_url and secret_token.
//  2. Approved creator with a synthetic Telegram identity.
//  3. POST /campaigns/{id}/creators (A1) — attach the creator to the campaign.
//  4. POST /campaigns/{id}/notify (A4) — flip the campaign_creator row to
//     status=invited so the TMA decision endpoints accept it.
//
// All cleanups are registered LIFO (campaign_creator → creator → application
// → campaign) so the FK chain unwinds without orphan rows. Registers the
// synthetic chat_id with the spy via SetupApprovedCreator so the notify call
// runs against the spy-only path even on a real bot deployment.
func SetupCampaignWithInvitedCreator(t *testing.T) TmaCampaignFixture {
	t.Helper()

	c, adminToken, _ := SetupAdminClient(t)
	uniqID := uuid.NewString()
	tmaToken := "tma_" + uniqID
	tmaURL := "https://tma.ugcboost.kz/tz/" + tmaToken

	campaignName := "Promo-" + uniqID
	createResp, err := c.CreateCampaignWithResponse(context.Background(),
		apiclient.CreateCampaignJSONRequestBody{Name: campaignName, TmaUrl: tmaURL},
		WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, createResp.StatusCode(),
		"SetupCampaignWithInvitedCreator: campaign create %d: %s",
		createResp.StatusCode(), string(createResp.Body))
	require.NotNil(t, createResp.JSON201)
	campaignID := createResp.JSON201.Data.Id.String()
	RegisterCampaignCleanup(t, campaignID)

	suffix := UniqueIIN()[6:]
	creator := SetupApprovedCreator(t, CreatorApplicationFixture{
		Socials: []SocialFixture{
			{Platform: "instagram", Handle: "tmaflowig" + suffix, Verification: VerificationAutoIG},
			{Platform: "tiktok", Handle: "tmaflowtt" + suffix, Verification: VerificationNone},
		},
	})

	campUUID := uuid.MustParse(campaignID)
	creatorUUID := uuid.MustParse(creator.CreatorID)
	addResp, err := c.AddCampaignCreatorsWithResponse(context.Background(), campUUID,
		apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []openapiUUID{creatorUUID}},
		WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, addResp.StatusCode(),
		"SetupCampaignWithInvitedCreator: A1 add %d: %s",
		addResp.StatusCode(), string(addResp.Body))
	require.NotNil(t, addResp.JSON201)
	require.Len(t, addResp.JSON201.Data.Items, 1, "A1 must return one campaign_creator row")
	campaignCreatorID := addResp.JSON201.Data.Items[0].Id.String()
	RegisterCampaignCreatorCleanup(t, c, adminToken, campaignID, creator.CreatorID)

	notifyResp, err := c.NotifyCampaignCreatorsWithResponse(context.Background(), campUUID,
		apiclient.NotifyCampaignCreatorsJSONRequestBody{CreatorIds: []openapiUUID{creatorUUID}},
		WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, notifyResp.StatusCode(),
		"SetupCampaignWithInvitedCreator: A4 notify %d: %s",
		notifyResp.StatusCode(), string(notifyResp.Body))

	// Wait for spy to record the notify message — protects e2e from a race
	// where the next assertion fires before the async send completes.
	_ = WaitForTelegramSent(t, creator.TelegramUserID, TelegramSentOptions{
		ExpectCount: 1,
		Timeout:     5 * time.Second,
	})

	return TmaCampaignFixture{
		CampaignID:        campaignID,
		CreatorID:         creator.CreatorID,
		CampaignCreatorID: campaignCreatorID,
		SecretToken:       tmaToken,
		TmaURL:            tmaURL,
		TelegramUserID:    creator.TelegramUserID,
		AdminToken:        adminToken,
		AdminClient:       c,
	}
}

// openapiUUID is the strict-server UUID alias used across the apiclient
// payloads. Tiny local alias keeps the helper imports clean.
type openapiUUID = uuid.UUID
