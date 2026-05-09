package testutil

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// FreshValidTmaURL returns a deterministically valid tma_url whose last path
// segment is unique per call. Required because campaigns.secret_token has a
// partial UNIQUE INDEX (live, non-deleted) and parallel tests would otherwise
// trip TMA_URL_CONFLICT when both seed against the same constant URL.
func FreshValidTmaURL() string {
	return "https://tma.ugcboost.kz/tz/" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// SetupCampaignWithContractTemplate seeds a fresh campaign and uploads a
// valid contract-template PDF so subsequent tests can exercise GET / replace
// / hasContractTemplate paths without rebuilding the upload flow each time.
// Uses the supplied admin token + apiclient for the create call and
// PutContractTemplate for the raw upload. The returned id has its cleanup
// already registered. Call after SetupAdminClient.
func SetupCampaignWithContractTemplate(t *testing.T, c *apiclient.ClientWithResponses, adminToken, name string) (campaignID string) {
	t.Helper()
	tma := FreshValidTmaURL()
	create, err := c.CreateCampaignWithResponse(context.Background(),
		apiclient.CreateCampaignJSONRequestBody{Name: name, TmaUrl: tma},
		WithAuth(adminToken))
	if err != nil {
		t.Fatalf("SetupCampaignWithContractTemplate.CreateCampaign: %v", err)
	}
	if create.StatusCode() != http.StatusCreated || create.JSON201 == nil {
		t.Fatalf("SetupCampaignWithContractTemplate.CreateCampaign: unexpected status %d", create.StatusCode())
	}
	id := create.JSON201.Data.Id
	RegisterCampaignCleanup(t, id.String())

	resp := PutContractTemplate(t, "/campaigns/"+id.String()+"/contract-template",
		BuildValidContractPDF(t), WithHeader("Authorization", "Bearer "+adminToken))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SetupCampaignWithContractTemplate.PutContractTemplate: unexpected status %d", resp.StatusCode)
	}
	return id.String()
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
