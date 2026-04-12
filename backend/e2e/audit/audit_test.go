package audit

// audit_test.go contains E2E tests for audit log recording, filtering, and access control.

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// --- Audit Log E2E ---

func TestAuditLogs_RecordedOnBrandCreate(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	name := "AuditBrand-" + testutil.UniqueEmail("audit-create")

	// Create a brand to generate an audit log entry
	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: name,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	brandID := resp.JSON201.Data.Id

	// List audit logs filtered by entity
	entityType := "brand"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		EntityType: &entityType,
		EntityId:   &brandID,
	}, testutil.WithAuth(token))
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
	c, token := testutil.LoginAsAdmin(t)
	brandID := testutil.SeedBrand(t, "AuditMgr-"+testutil.UniqueEmail("audit-mgr"))
	email := testutil.UniqueEmail("audit-mgr-user")

	_, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: email,
	}, testutil.WithAuth(token))
	require.NoError(t, err)

	entityType := "brand"
	action := "manager_assign"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		EntityType: &entityType,
		EntityId:   &brandID,
		Action:     &action,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)
	require.Greater(t, len(logsResp.JSON200.Data.Logs), 0, "manager_assign audit log should exist")
}

func TestAuditLogs_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	c, token, _, _ := testutil.LoginAsBrandManager(t)

	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, logsResp.StatusCode())
}

func TestAuditLogs_Pagination(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)

	// Create multiple brands to generate audit entries
	for i := 0; i < 3; i++ {
		testutil.SeedBrand(t, "PagBrand-"+testutil.UniqueEmail("pag"))
	}

	perPage := 2
	page := 1
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		Page:    &page,
		PerPage: &perPage,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)
	require.LessOrEqual(t, len(logsResp.JSON200.Data.Logs), 2)
	require.Greater(t, logsResp.JSON200.Data.Total, 0)
}

func TestAuditLogs_FilterByAction(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	testutil.SeedBrand(t, "FilterBrand-"+testutil.UniqueEmail("filter"))

	action := "brand_create"
	logsResp, err := c.ListAuditLogsWithResponse(context.Background(), &apiclient.ListAuditLogsParams{
		Action: &action,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, logsResp.StatusCode())
	require.NotNil(t, logsResp.JSON200)

	for _, log := range logsResp.JSON200.Data.Logs {
		require.Equal(t, "brand_create", log.Action)
	}
}
