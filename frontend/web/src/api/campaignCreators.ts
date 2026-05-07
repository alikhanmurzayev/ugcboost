import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CampaignCreator = components["schemas"]["CampaignCreator"];

function extractErrorCode(error: unknown): string {
  if (
    typeof error === "object" &&
    error !== null &&
    "error" in error &&
    typeof (error as { error: unknown }).error === "object" &&
    (error as { error: unknown }).error !== null &&
    "code" in (error as { error: { code?: unknown } }).error
  ) {
    const code = (error as { error: { code?: unknown } }).error.code;
    if (typeof code === "string") return code;
  }
  return "INTERNAL_ERROR";
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
