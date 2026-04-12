package e2etest

// audit_test.go contains E2E tests for audit log recording, filtering, and access control.

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/e2etest/apiclient"
)

// --- Audit Log E2E ---

func TestAuditLogs_RecordedOnBrandCreate(t *testing.T) {
	t.Parallel()
	c, token := loginAsAdmin(t)
	name := "AuditBrand-" + uniqueEmail("audit-create")

	// Create a brand to generate an audit log entry
	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: name,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	brandID := resp.JSON201.Data.Id

	// List audit logs filtered by entity
	entityType := "brand"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		EntityType: &entityType,
		EntityId:   &brandID,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)

	found := false
	for _, log := range logsResp.JSON200.Data.Logs {
		if log.Action == "brand_create" {
			found = true
			require.Equal(t, "brand", log.EntityType)
			break
		}
	}
	require.True(t, found, "brand_create audit log should exist")
}

func TestAuditLogs_RecordedOnManagerAssign(t *testing.T) {
	t.Parallel()
	c, token := loginAsAdmin(t)
	brandID := seedBrand(t, "AuditMgr-"+uniqueEmail("audit-mgr"))
	email := uniqueEmail("audit-mgr-user")

	_, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: email,
	}, withAuth(token))
	require.NoError(t, err)

	entityType := "brand"
	action := "manager_assign"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		EntityType: &entityType,
		EntityId:   &brandID,
		Action:     &action,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)
	require.Greater(t, len(logsResp.JSON200.Data.Logs), 0, "manager_assign audit log should exist")
}

func TestAuditLogs_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	c, token, _, _ := loginAsBrandManager(t)

	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, logsResp.StatusCode())
}

func TestAuditLogs_Pagination(t *testing.T) {
	t.Parallel()
	c, token := loginAsAdmin(t)

	// Create multiple brands to generate audit entries
	for i := 0; i < 3; i++ {
		seedBrand(t, "PagBrand-"+uniqueEmail("pag"))
	}

	perPage := 2
	page := 1
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		Page:    &page,
		PerPage: &perPage,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)
	require.LessOrEqual(t, len(logsResp.JSON200.Data.Logs), 2)
	require.Greater(t, logsResp.JSON200.Data.Total, 0)
}

func TestAuditLogs_FilterByAction(t *testing.T) {
	t.Parallel()
	c, token := loginAsAdmin(t)
	seedBrand(t, "FilterBrand-"+uniqueEmail("filter"))

	action := "brand_create"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		Action: &action,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)

	for _, log := range logsResp.JSON200.Data.Logs {
		require.Equal(t, "brand_create", log.Action)
	}
}
