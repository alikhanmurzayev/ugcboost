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
// Telegram-блок и hydrated city/category names.
//
// Все t.Run параллельны. Cleanup-стек регистрирует creator перед
// родительской заявкой (LIFO) — FK creators.source_application_id не
// несёт ON DELETE, родителя нельзя удалить, пока дочерний ряд жив.
package creators_test

import (
	"context"
	"net/http"
	"testing"

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
			testutil.WithAuth(fx.AdminToken))
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
}
