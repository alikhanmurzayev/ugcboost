import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CampaignCreator = components["schemas"]["CampaignCreator"];
export type AddCampaignCreatorsInput =
  components["schemas"]["AddCampaignCreatorsInput"];

function extractErrorParts(error: unknown): {
  code: string;
  message?: string;
} {
  if (
    typeof error === "object" &&
    error !== null &&
    "error" in error &&
    typeof (error as { error: unknown }).error === "object" &&
    (error as { error: unknown }).error !== null
  ) {
    const inner = (error as { error: { code?: unknown; message?: unknown } })
      .error;
    const code = typeof inner.code === "string" ? inner.code : "INTERNAL_ERROR";
    const message =
      typeof inner.message === "string" ? inner.message : undefined;
    return { code, message };
  }
  return { code: "INTERNAL_ERROR" };
}

export async function listCampaignCreators(
  campaignId: string,
): Promise<CampaignCreator[]> {
  const { data, error, response } = await client.GET(
    "/campaigns/{id}/creators",
    { params: { path: { id: campaignId } } },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(response.status, parts.code, parts.message);
  }
  return data.data.items;
}

export async function addCampaignCreators(
  campaignId: string,
  creatorIds: string[],
): Promise<CampaignCreator[]> {
  const { data, error, response } = await client.POST(
    "/campaigns/{id}/creators",
    {
      params: { path: { id: campaignId } },
      body: { creatorIds },
    },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(response.status, parts.code, parts.message);
  }
  return data.data.items;
}

export async function removeCampaignCreator(
  campaignId: string,
  creatorId: string,
): Promise<void> {
  const { error, response } = await client.DELETE(
    "/campaigns/{id}/creators/{creatorId}",
    { params: { path: { id: campaignId, creatorId } } },
  );
  if (error) {
    const parts = extractErrorParts(error);
    throw new ApiError(response.status, parts.code, parts.message);
  }
}
