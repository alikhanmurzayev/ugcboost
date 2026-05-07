import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CampaignCreator = components["schemas"]["CampaignCreator"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function listCampaignCreators(
  campaignId: string,
): Promise<CampaignCreator[]> {
  const { data, error, response } = await client.GET(
    "/campaigns/{id}/creators",
    { params: { path: { id: campaignId } } },
  );
  if (error || !data) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data.data.items;
}
