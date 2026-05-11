// Package creators — E2E HTTP-поверхность GET /creators/{id} (chunk 18c
// onboarding-roadmap'а): admin-only aggregate read, который собирает
// creator-row + snapshot socials + snapshot categories + локализованные
// имена через словарь. Сценарии покрывают всю I/O-матрицу: 401 при
// отсутствии Bearer'а, 403 при brand_manager-токене, 404 при random UUID,
// и happy path через переиспользуемый fixture-pipeline
// SetupCreatorApplicationInModeration: после submit→link→IG-auto-verify
// заявка переходит в moderation, тестовый admin одобряет её и читает
// агрегат. AssertCreatorAggregateMatchesSetup сверяет ответ с фикстурой
// поле-в-поле — sorted socials по (platform, handle), sorted categories
// по code, dynamic поля (id, timestamps, social.id, social.verifiedAt)
// валидируются через WithinDuration и подменяются на observed значения
// перед structural equality. Один happy-сценарий покрывает identity, PII
// (включая nullable middle_name/address/category_other_text), плоский
// Telegram-блок и hydrated city/category names; helper заодно требует
// пустой `campaigns` для свежеаппрувленных креаторов без participations.
//
// Отдельный сценарий описывает поле `campaigns`: креатора прикрепляют к
// двум живым кампаниям и одной soft-deleted, после чего ответ агрегата
// проверяется на (1) исключение soft-deleted, (2) DESC-порядок по
// `campaign_creators.created_at` (бэкенд гарантирует его, фронт его
// сохраняет внутри групп) и (3) корректный маппинг id/name/status в
// CreatorCampaignBrief.
//
// Все t.Run параллельны. Cleanup-стек регистрирует creator перед
// родительской заявкой (LIFO) — FK creators.source_application_id не
// несёт ON DELETE, родителя нельзя удалить, пока дочерний ряд жив.
package creators_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestGetCreator(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorWithResponse(context.Background(), uuid.New())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"creator-get-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorWithResponse(context.Background(), uuid.New(),
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("creator not found returns 404 CREATOR_NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorWithResponse(context.Background(), uuid.New(),
			testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "CREATOR_NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("happy — full aggregate after approve, structural match against fixture", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			MiddleName:        pointer.ToString("Ивановна"),
			Address:           pointer.ToString("ул. Абая 10"),
			CategoryCodes:     []string{"beauty", "fashion", "other"},
			CategoryOtherText: pointer.ToString("стримы"),
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "aidana_get", Verification: testutil.VerificationAutoIG},
				{Platform: string(apiclient.Tiktok), Handle: "aidana_tt_get", Verification: testutil.VerificationNone},
				{Platform: string(apiclient.Threads), Handle: "aidana_th_get", Verification: testutil.VerificationNone},
			},
		})

		c := testutil.NewAPIClient(t)
		appUUID := uuid.MustParse(fx.ApplicationID)
		approveResp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, approveResp.StatusCode())
		require.NotNil(t, approveResp.JSON200)
		creatorID := approveResp.JSON200.Data.CreatorId
		require.NotEqual(t, uuid.Nil, creatorID)
		testutil.RegisterCreatorCleanup(t, creatorID.String())

		getResp, err := c.GetCreatorWithResponse(context.Background(), creatorID,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)
		testutil.AssertCreatorAggregateMatchesSetup(t, fx, creatorID.String(), getResp.JSON200.Data)
	})

	t.Run("happy — campaigns excludes soft-deleted and orders by created_at DESC", func(t *testing.T) {
		t.Parallel()
		seeded := testutil.SetupApprovedCreator(t, testutil.CreatorApplicationFixture{
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "campget_" + testutil.UniqueIIN()[6:], Verification: testutil.VerificationAutoIG},
				{Platform: string(apiclient.Tiktok), Handle: "campget_tt_" + testutil.UniqueIIN()[6:], Verification: testutil.VerificationNone},
			},
		})

		c := testutil.NewAPIClient(t)
		suffix := testutil.UniqueIIN()[6:]
		camp1 := testutil.SetupCampaign(t, c, seeded.AdminToken, "ccget-1-"+suffix)
		testutil.AttachCreatorToCampaign(t, c, seeded.AdminToken, camp1, seeded.CreatorID)

		// Postgres now() is microsecond-precision, so back-to-back inserts on
		// the same connection are virtually always distinct, but a 2-ms pause
		// makes the (created_at DESC, id DESC) ordering assertion below robust
		// against the corner case where two timestamps collide and id (random
		// UUID) becomes the tiebreaker — which would be unrelated to attach
		// order.
		time.Sleep(2 * time.Millisecond)
		camp2 := testutil.SetupCampaign(t, c, seeded.AdminToken, "ccget-2-"+suffix)
		testutil.AttachCreatorToCampaign(t, c, seeded.AdminToken, camp2, seeded.CreatorID)

		time.Sleep(2 * time.Millisecond)
		camp3 := testutil.SetupCampaign(t, c, seeded.AdminToken, "ccget-3-"+suffix)
		testutil.AttachCreatorToCampaign(t, c, seeded.AdminToken, camp3, seeded.CreatorID)
		testutil.SoftDeleteCampaign(t, camp3)

		creatorUUID := uuid.MustParse(seeded.CreatorID)
		getResp, err := c.GetCreatorWithResponse(context.Background(), creatorUUID,
			testutil.WithAuth(seeded.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)

		got := getResp.JSON200.Data.Campaigns
		require.Len(t, got, 2, "soft-deleted campaign must be excluded")
		ids := []string{got[0].Id.String(), got[1].Id.String()}
		require.NotContains(t, ids, camp3, "soft-deleted campaign id must not appear")
		// camp2 was attached AFTER camp1, so it should come first under DESC order.
		require.Equal(t, camp2, got[0].Id.String(), "second attached campaign must come first under created_at DESC")
		require.Equal(t, camp1, got[1].Id.String(), "first attached campaign must come second under created_at DESC")
		// Status defaults to `planned` for a fresh attach without notify/decision.
		require.Equal(t, apiclient.Planned, got[0].Status)
		require.Equal(t, apiclient.Planned, got[1].Status)
		require.Equal(t, "ccget-2-"+suffix, got[0].Name)
		require.Equal(t, "ccget-1-"+suffix, got[1].Name)
	})
}
