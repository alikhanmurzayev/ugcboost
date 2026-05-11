package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlekSi/pointer"
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

// ListAuditEntriesByAction returns every audit log entry for the given
// (entityType, entityID) whose `action` matches. Useful for asserting
// idempotent flows: a TMA agree must write exactly one row, and a repeat
// agree must NOT add a second one.
func ListAuditEntriesByAction(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string) []apiclient.AuditLogEntry {
	t.Helper()
	resp, err := c.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			EntityType: &entityType,
			EntityId:   &entityID,
		}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	out := make([]apiclient.AuditLogEntry, 0, len(resp.JSON200.Data.Logs))
	for _, l := range resp.JSON200.Data.Logs {
		if l.Action == action {
			out = append(out, l)
		}
	}
	return out
}

// AuditValueMap decodes an audit_logs JSON snapshot (OldValue / NewValue —
// both surfaced as `any` by the generated AuditLogEntry) into a generic
// map[string]any so individual fields can be asserted without importing
// internal/domain (e2e is a separate module). Fails the test if raw is nil
// or cannot be remarshaled into a map.
func AuditValueMap(t *testing.T, raw interface{}) map[string]any {
	t.Helper()
	require.NotNil(t, raw)
	payload, err := json.Marshal(raw)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(payload, &m))
	return m
}

// CountAuditEntries returns the number of audit rows matching
// (entityType, entityID, action). Caller pins entityID to a fresh fixture
// row, so the per-row count never realistically nears the per_page cap —
// the helper still asserts `got < perPage` to fail loudly if a future test
// floods the same fixture id with audit rows and silently truncates.
func CountAuditEntries(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, entityType, entityID, action string,
) int {
	t.Helper()
	const perPage = 100
	resp, err := c.ListAuditLogsWithResponse(context.Background(),
		&apiclient.ListAuditLogsParams{
			Page:       pointer.ToInt(1),
			PerPage:    pointer.ToInt(perPage),
			EntityType: pointer.ToString(entityType),
			EntityId:   pointer.ToString(entityID),
			Action:     pointer.ToString(action),
		},
		WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	got := len(resp.JSON200.Data.Logs)
	require.Lessf(t, got, perPage,
		"audit count reached per_page=%d cap — assertion would silently truncate; paginate or narrow filters",
		perPage)
	return got
}
