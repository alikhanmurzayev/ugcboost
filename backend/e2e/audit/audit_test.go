// Package audit — E2E тесты HTTP-поверхности GET /audit-logs.
//
// TestAuditLogFiltering проверяет фильтры и пагинацию листинга. Фильтр по
// сущности (entity_type=brand + entity_id=brandID) после POST /brands
// находит запись brand_create, привязанную к этому же id — значит аудит
// действительно пишется в той же транзакции, что и создание бренда, и
// пишет правильный entity_id. Фильтр по action=manager_assign после
// POST /brands/{id}/managers обнаруживает ровно запись об ассайне.
// Пагинация проверяется на трёх созданных админом брендах: per_page=2
// урезает возвращаемый срез до двух элементов, а total продолжает считать
// каждую подходящую строку (≥ 3) — это и есть контракт пагинации.
// Фильтрация по actor_id в этом сценарии важна отдельно: параллельно
// бегущие тесты могут создавать свои audit-ряды, и без пина к actor_id
// ассерт total получился бы флаки.
//
// TestAuditLogAccess закрывает авторизацию. Админский GET — 200 с валидным
// листингом, brand_manager получает 403 с кодом FORBIDDEN: аудит — только
// для админов, и этот инвариант закрыт на уровне e2e.
//
// Сетап идёт через testutil.Setup* — все созданные пользователи и бренды
// снимаются автоматически через cleanup stack. Сами аудит-ряды удаляются
// косвенно: при hard-delete актёра DeleteForTests в одной транзакции
// вычищает audit_logs WHERE actor_id = $1 — так рост таблицы аудита не
// переживает тестовые прогоны.
package audit

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestAuditLogFiltering(t *testing.T) {
	t.Parallel()

	t.Run("filter by entity captures brand_create", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "AuditEntity-"+testutil.UniqueEmail("audit-entity"))

		entityType := "brand"
		resp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
			EntityType: &entityType,
			EntityId:   &brandID,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		require.True(t, testutil.ContainsAction(resp.JSON200.Data.Logs, "brand_create"),
			"brand_create audit entry must be present for the new brand")
		for _, log := range resp.JSON200.Data.Logs {
			require.Equal(t, "brand", log.EntityType)
			require.NotNil(t, log.EntityId)
			require.Equal(t, brandID, *log.EntityId)
		}
	})

	t.Run("filter by action=manager_assign after assign", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "AuditAction-"+testutil.UniqueEmail("audit-action"))
		testutil.SetupManager(t, c, token, brandID)

		entityType := "brand"
		action := "manager_assign"
		resp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
			EntityType: &entityType,
			EntityId:   &brandID,
			Action:     &action,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.NotEmpty(t, resp.JSON200.Data.Logs, "at least one manager_assign entry expected")
		for _, log := range resp.JSON200.Data.Logs {
			require.Equal(t, "manager_assign", log.Action)
		}
	})

	t.Run("pagination caps page size while total reflects every match", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		// Create three brands so this admin has at least three matching
		// audit rows. Scoping by actor_id guarantees isolation from other
		// parallel tests that may be spawning audit rows of their own.
		testutil.SetupBrand(t, c, token, "Pag1-"+testutil.UniqueEmail("pag1"))
		testutil.SetupBrand(t, c, token, "Pag2-"+testutil.UniqueEmail("pag2"))
		testutil.SetupBrand(t, c, token, "Pag3-"+testutil.UniqueEmail("pag3"))

		actorID := adminUserID(t, c, token)
		action := "brand_create"
		perPage := 2
		page := 1
		resp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
			ActorId: &actorID,
			Action:  &action,
			Page:    &page,
			PerPage: &perPage,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		require.Len(t, resp.JSON200.Data.Logs, 2,
			"per_page=2 must trim the returned slice to 2 entries")
		require.GreaterOrEqual(t, resp.JSON200.Data.Total, 3,
			"total must count every matching brand_create row this admin produced")
		require.Equal(t, page, resp.JSON200.Data.Page)
		require.Equal(t, perPage, resp.JSON200.Data.PerPage)
	})
}

func TestAuditLogAccess(t *testing.T) {
	t.Parallel()

	t.Run("admin GET returns 200", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)

		resp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
	})

	t.Run("brand_manager GET returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "AuditMgr-"+testutil.UniqueEmail("auditmgr"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		resp, err := mgrClient.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})
}

// adminUserID resolves the current admin's user ID via GET /auth/me. Tests
// that need to filter audit logs by actor_id use this to pin the filter to
// their own seeded admin, keeping assertions isolated from other parallel
// tests that might be creating audit rows concurrently.
func adminUserID(t *testing.T, c *apiclient.ClientWithResponses, token string) string {
	t.Helper()
	resp, err := c.GetMeWithResponse(context.Background(), testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return resp.JSON200.Data.Id
}
