package testutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
)

// RegisterCampaignCreatorCleanup schedules a DELETE /campaigns/{id}/creators/{creatorId}
// (admin) for the given pair after the test. 404 is treated as success — the
// row may have been removed already by an inline assertion or sibling
// cleanup. The campaign_creators FK to campaigns has no ON DELETE CASCADE, so
// LIFO cleanup order MUST remove campaign_creators rows before the parent
// campaign / creator rows; tests register this AFTER the parent
// RegisterCampaignCleanup / RegisterCreatorCleanup so it fires first.
func RegisterCampaignCreatorCleanup(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, campaignID, creatorID string) {
	t.Helper()
	campUUID, err := uuid.Parse(campaignID)
	if err != nil {
		t.Fatalf("RegisterCampaignCreatorCleanup: invalid campaign id %q: %v", campaignID, err)
	}
	creatorUUID, err := uuid.Parse(creatorID)
	if err != nil {
		t.Fatalf("RegisterCampaignCreatorCleanup: invalid creator id %q: %v", creatorID, err)
	}
	RegisterCleanup(t, func(ctx context.Context) error {
		resp, err := c.RemoveCampaignCreatorWithResponse(ctx, campUUID, creatorUUID, WithAuth(adminToken))
		if err != nil {
			return fmt.Errorf("cleanup campaign_creator (%s, %s): %w", campaignID, creatorID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("cleanup campaign_creator (%s, %s): unexpected status %d",
				campaignID, creatorID, resp.StatusCode())
		}
		return nil
	})
}
