package testutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// SetupCampaign creates a campaign through POST /campaigns using adminToken
// and returns its UUID. Cleanup runs through POST /test/cleanup-entity (no
// business DELETE endpoint exists yet — chunk #7 will introduce soft-delete;
// this hard-delete via testapi is intentionally test-only).
func SetupCampaign(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name, tmaURL string) string {
	t.Helper()
	resp, err := c.CreateCampaignWithResponse(context.Background(), apiclient.CreateCampaignJSONRequestBody{
		Name:   name,
		TmaUrl: tmaURL,
	}, WithAuth(adminToken))
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode(),
		"SetupCampaign: create must return 201, got %d", resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	id := resp.JSON201.Data.Id.String()
	RegisterCampaignCleanup(t, id)
	return id
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
