package testutil

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// FreshValidTmaURL returns a deterministically valid tma_url whose last path
// segment is unique per call. Required because campaigns.secret_token has a
// partial UNIQUE INDEX (live, non-deleted) and parallel tests would otherwise
// trip TMA_URL_CONFLICT when both seed against the same constant URL.
func FreshValidTmaURL() string {
	return "https://tma.ugcboost.kz/tz/" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

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
