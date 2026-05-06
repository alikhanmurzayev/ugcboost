package testutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// RegisterCampaignCleanup schedules a POST /test/cleanup-entity for a
// campaign row after the test. 404 is treated as success — the row may have
// been removed already by another step or by a sibling cleanup. Cleanups are
// LIFO via t.Cleanup, so registering this AFTER any child rows ensures FK
// constraints still hold (today campaigns has no children — kept for future
// chunks #10+ that introduce campaign_creators).
func RegisterCampaignCleanup(t *testing.T, campaignID string) {
	t.Helper()
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.Campaign,
			Id:   campaignID,
		})
		if err != nil {
			return fmt.Errorf("cleanup campaign %s: %w", campaignID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup campaign %s: unexpected status %d", campaignID, resp.StatusCode())
		}
		return nil
	})
}
