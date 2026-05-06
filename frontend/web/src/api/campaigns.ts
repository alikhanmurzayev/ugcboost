import type { components, paths } from "./generated/schema";
import client, { ApiError } from "./client";

export type Campaign = components["schemas"]["Campaign"];
export type CampaignListSortField = components["schemas"]["CampaignListSortField"];
export type CampaignsListData = components["schemas"]["CampaignsListData"];
export type CampaignsListInput = paths["/campaigns"]["get"]["parameters"]["query"];
export type CampaignInput = components["schemas"]["CampaignInput"];
export type CampaignCreatedResult = components["schemas"]["CampaignCreatedResult"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function listCampaigns(input: CampaignsListInput) {
  const { data, error, response } = await client.GET("/campaigns", {
    params: { query: input },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function createCampaign(
  input: CampaignInput,
): Promise<CampaignCreatedResult> {
  const { data, error, response } = await client.POST("/campaigns", {
    body: input,
  });
  if (error || !data) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}
