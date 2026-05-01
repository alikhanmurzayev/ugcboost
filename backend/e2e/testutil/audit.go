package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// FindAuditEntry fetches audit logs filtered by (entityType, entityID) using
// the admin token, then returns the first entry matching the given action.
// Fails the test if the entry is not found — callers can then assert on
// any field (ipAddress, actorRole, etc.) without re-running the lookup.
func FindAuditEntry(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string) *apiclient.AuditLogEntry {
	t.Helper()
	resp, err := c.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			EntityType: &entityType,
			EntityId:   &entityID,
		}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	for i := range resp.JSON200.Data.Logs {
		entry := &resp.JSON200.Data.Logs[i]
		if entry.Action == action {
			return entry
		}
	}
	t.Fatalf("expected audit entry action=%q for %s/%s, got %d entries",
		action, entityType, entityID, len(resp.JSON200.Data.Logs))
	return nil
}

// AssertAuditEntry checks that an audit row matching (entityType, entityID,
// action) exists. Tests that need exhaustive verification of fields beyond
// presence should use FindAuditEntry directly.
func AssertAuditEntry(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string) {
	t.Helper()
	_ = FindAuditEntry(t, c, adminToken, entityType, entityID, action)
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
