// Package creator_applications — E2E тесты HTTP-поверхности
// GET /creators/applications/counts (admin-only, чанк 5 onboarding-roadmap'а).
//
// TestCreatorApplicationsCounts покрывает контур ручки в одной функции через
// t.Run в порядке исполнения: сначала границы доступа (отсутствие Bearer'а
// отдаёт 401, brand_manager — 403 без leak'а реальных значений), затем smoke
// happy-path — после реальной подачи заявки через лендинг админ должен
// увидеть в ответе элемент {status: "verification", count >= 1}. Остальные
// статусы появляются по мере подключения соответствующих переходов; ручка
// возвращает **sparse** список — статусы без рядов в БД отсутствуют, фронт
// лукапит `find(c => c.status === STATUS_X)?.count ?? 0`.
//
// Тесты идемпотентны и параллельны: каждый t.Run создаёт собственную заявку
// через testutil.SetupCreatorApplicationViaLanding, так что параллельные
// прогоны не делят данные. Empty-DB сценарий не покрывается на e2e —
// невозможно гарантировать пустую БД пока другие тесты создают строки в
// параллели; этот случай покрыт unit-тестом сервиса.
package creator_applications_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestCreatorApplicationsCounts(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		// No Authorization header — middleware rejects with 401 before any
		// business logic.
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorApplicationsCountsWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "counts-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorApplicationsCountsWithResponse(context.Background(), testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("admin smoke: verification count >= 1 after a single submit", func(t *testing.T) {
		t.Parallel()
		// Submit one application via the public landing endpoint so we know at
		// least one row in status=verification exists. Other statuses can grow
		// in parallel from neighbouring tests, but verification must be present
		// here regardless.
		testutil.SetupCreatorApplicationViaLanding(t)

		_, adminToken, _ := testutil.SetupAdminClient(t)
		c := testutil.NewAPIClient(t)
		resp, err := c.GetCreatorApplicationsCountsWithResponse(context.Background(), testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		items := resp.JSON200.Data.Items
		// Sparse contract: every item must reference a non-zero positive count
		// for a known status. Statuses with zero rows are omitted entirely.
		seen := make(map[apiclient.CreatorApplicationStatus]int64, len(items))
		for _, item := range items {
			require.Greater(t, item.Count, int64(0),
				"sparse counts must not include zero entries — status %s came back with count=%d", item.Status, item.Count)
			seen[item.Status] = item.Count
		}
		require.Contains(t, seen, apiclient.Verification,
			"verification must appear in counts after a successful submission")
		require.GreaterOrEqual(t, seen[apiclient.Verification], int64(1))

		// Sparse + alphabetical: assert ordering on the wire so the frontend
		// can rely on it without re-sorting.
		for i := 1; i < len(items); i++ {
			require.Less(t, string(items[i-1].Status), string(items[i].Status),
				"items must be sorted alphabetically by status; got %v", items)
		}
	})
}
