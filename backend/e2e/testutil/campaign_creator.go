package testutil

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// AttachCreatorToCampaign POST /campaigns/{id}/creators batches a single
// creatorID, asserts 201 and registers the matching campaign_creator cleanup
// in the LIFO stack. Returns nothing — the (campaign, creator) pair is the
// caller-visible identifier and is already known at the call site.
func AttachCreatorToCampaign(t *testing.T, c *apiclient.ClientWithResponses,
	adminToken, campaignID, creatorID string) {
	t.Helper()
	campUUID, err := uuid.Parse(campaignID)
	if err != nil {
		t.Fatalf("AttachCreatorToCampaign: invalid campaign id %q: %v", campaignID, err)
	}
	creatorUUID, err := uuid.Parse(creatorID)
	if err != nil {
		t.Fatalf("AttachCreatorToCampaign: invalid creator id %q: %v", creatorID, err)
	}
	resp, err := c.AddCampaignCreatorsWithResponse(context.Background(), campUUID,
		apiclient.AddCampaignCreatorsJSONRequestBody{CreatorIds: []uuid.UUID{creatorUUID}},
		WithAuth(adminToken))
	if err != nil {
		t.Fatalf("AttachCreatorToCampaign: request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("AttachCreatorToCampaign: unexpected status %d, body=%s", resp.StatusCode(), string(resp.Body))
	}
	RegisterCampaignCreatorForceCleanup(t, campaignID, creatorID)
}

// RegisterCampaignCreatorForceCleanup schedules a hard-delete of the
// (campaign_id, creator_id) row through the test-only force-cleanup endpoint.
// Use this in scenarios where the parent campaign may be flipped to
// `is_deleted = true` mid-test — the production DELETE
// /campaigns/{id}/creators/{creatorId} endpoint refuses to operate on
// soft-deleted campaigns, which leaks the row and breaks the downstream
// `cleanup-entity` for the campaign itself.
func RegisterCampaignCreatorForceCleanup(t *testing.T, campaignID, creatorID string) {
	t.Helper()
	campUUID, err := uuid.Parse(campaignID)
	if err != nil {
		t.Fatalf("RegisterCampaignCreatorForceCleanup: invalid campaign id %q: %v", campaignID, err)
	}
	creatorUUID, err := uuid.Parse(creatorID)
	if err != nil {
		t.Fatalf("RegisterCampaignCreatorForceCleanup: invalid creator id %q: %v", creatorID, err)
	}
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.ForceCleanupCampaignCreatorWithResponse(ctx,
			testclient.ForceCleanupCampaignCreatorJSONRequestBody{
				CampaignId: campUUID,
				CreatorId:  creatorUUID,
			})
		if err != nil {
			return fmt.Errorf("force-cleanup campaign_creator (%s, %s): %w", campaignID, creatorID, err)
		}
		if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusNotFound {
			return fmt.Errorf("force-cleanup campaign_creator (%s, %s): unexpected status %d",
				campaignID, creatorID, resp.StatusCode())
		}
		return nil
	})
}

// RegisterCampaignCreatorCleanup schedules a hard-delete of the (campaign_id,
// creator_id) row via POST /test/cleanup-entity (type=campaign_creator). The
// production admin DELETE /campaigns/{id}/creators/{creatorId} refuses to
// operate on rows in terminal statuses (agreed / signing / signed / declined),
// so cleanup that goes through the production path silently leaks 422s and
// leaves rows in staging. The testapi endpoint hard-deletes without the
// business-layer guard. 404 is treated as success — the row may have been
// removed already by an inline assertion or sibling cleanup. The
// campaign_creators FK to campaigns has no ON DELETE CASCADE, so LIFO cleanup
// order MUST remove campaign_creators rows before the parent campaign /
// creator rows; tests register this AFTER the parent RegisterCampaignCleanup /
// RegisterCreatorCleanup so it fires first.
//
// Compound id: cleanup-entity dispatches by (campaign_id, creator_id) — the
// pair is what callers already know; the campaign_creators.id UUID is often
// not even read by the test (e.g. approve_with_campaigns flows).
func RegisterCampaignCreatorCleanup(t *testing.T, campaignID, creatorID string) {
	t.Helper()
	if _, err := uuid.Parse(campaignID); err != nil {
		t.Fatalf("RegisterCampaignCreatorCleanup: invalid campaign id %q: %v", campaignID, err)
	}
	if _, err := uuid.Parse(creatorID); err != nil {
		t.Fatalf("RegisterCampaignCreatorCleanup: invalid creator id %q: %v", creatorID, err)
	}
	RegisterCleanup(t, func(ctx context.Context) error {
		tc := NewTestClient(t)
		resp, err := tc.CleanupEntityWithResponse(ctx, testclient.CleanupEntityJSONRequestBody{
			Type: testclient.CampaignCreator,
			Id:   campaignID + ":" + creatorID,
		})
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
