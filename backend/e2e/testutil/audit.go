package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// AssertAuditEntry checks that an audit row matching (entityType, entityID,
// action) exists. Uses the admin token to call GET /audit-logs with the
// server-side filter, then asserts the action is present in the returned
// page. Tests that need exhaustive verification of fields beyond presence
// can read resp.JSON200 via the admin client directly.
func AssertAuditEntry(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string) {
	t.Helper()
	resp, err := c.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			EntityType: &entityType,
			EntityId:   &entityID,
		}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.True(t, ContainsAction(resp.JSON200.Data.Logs, action),
		"expected audit entry action=%q for %s/%s", action, entityType, entityID)
}

// ContainsAction reports whether logs contain at least one entry with the
// given action. Exported so audit/audit_test.go (and any future test) can
// share the same predicate.
func ContainsAction(logs []apiclient.AuditLogEntry, action string) bool {
	for _, l := range logs {
		if l.Action == action {
			return true
		}
	}
	return false
}
