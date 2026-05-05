package testutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// RegisterCreatorCleanup schedules a POST /test/cleanup-entity for a creator
// row after the test. The testapi handler delegates to CreatorRepo.DeleteForTests
// which cascades through creator_socials and creator_categories. 404 is treated
// as success — the row may have been removed by another step. Cleanups are
// stacked LIFO via t.Cleanup so registering this AFTER the parent application
// cleanup ensures the creator goes first; the FK creators.source_application_id
// has no ON DELETE clause, so the application cannot be deleted while a creator
// still references it.
func RegisterCreatorCleanup(t *testing.T, creatorID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.Creator,
			Id:   creatorID,
		})
		if err != nil {
			return fmt.Errorf("cleanup creator %s: %w", creatorID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup creator %s: unexpected status %d", creatorID, resp.StatusCode())
		}
		return nil
	})
}

// DeleteCreatorForTests calls POST /test/cleanup-entity once and asserts the
// row was actually present. Use this when a test wants to prove a creator-row
// was created (a 200/204 success means the testapi found and deleted exactly
// one row, a 404 surfaces as a require-failure here). For ordinary cleanup
// stack registration prefer RegisterCreatorCleanup, which silently treats
// 404 as success.
func DeleteCreatorForTests(t *testing.T, creatorID string) {
	t.Helper()
	tc := NewTestClient(t)
	resp, err := tc.CleanupEntityWithResponse(context.Background(), testclient.CleanupEntityJSONRequestBody{
		Type: testclient.Creator,
		Id:   creatorID,
	})
	if err != nil {
		t.Fatalf("delete creator %s: %v", creatorID, err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		t.Fatalf("delete creator %s: unexpected status %d", creatorID, resp.StatusCode())
	}
}
