import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CampaignCreator = components["schemas"]["CampaignCreator"];
export type CampaignCreatorStatus =
  components["schemas"]["CampaignCreatorStatus"];
export type AddCampaignCreatorsInput =
  components["schemas"]["AddCampaignCreatorsInput"];
export type CampaignNotifyResult =
  components["schemas"]["CampaignNotifyResult"];
export type CampaignNotifyUndelivered =
  components["schemas"]["CampaignNotifyUndelivered"];
export type CampaignNotifyUndeliveredReason =
  components["schemas"]["CampaignNotifyUndeliveredReason"];
export type CampaignCreatorBatchInvalidError =
  components["schemas"]["CampaignCreatorBatchInvalidError"];
export type CampaignCreatorBatchInvalidDetail =
  components["schemas"]["CampaignCreatorBatchInvalidDetail"];
export type CampaignCreatorPatchInput =
  components["schemas"]["CampaignCreatorPatchInput"];

function extractErrorParts(error: unknown): {
  code: string;
  message?: string;
  details?: unknown;
} {
  if (!isRecord(error)) return { code: "INTERNAL_ERROR" };
  const inner = error.error;
  if (!isRecord(inner)) return { code: "INTERNAL_ERROR" };
  const code =
    typeof inner.code === "string" ? inner.code : "INTERNAL_ERROR";
  const message =
    typeof inner.message === "string" ? inner.message : undefined;
  return { code, message, details: inner.details };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
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
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
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
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
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
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
  }
}

export async function notifyCampaignCreators(
  campaignId: string,
  creatorIds: string[],
): Promise<CampaignNotifyResult> {
  const { data, error, response } = await client.POST(
    "/campaigns/{id}/notify",
    {
      params: { path: { id: campaignId } },
      body: { creatorIds },
    },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
  }
  return data;
}

export async function remindCampaignCreatorsInvitation(
  campaignId: string,
  creatorIds: string[],
): Promise<CampaignNotifyResult> {
  const { data, error, response } = await client.POST(
    "/campaigns/{id}/remind-invitation",
    {
      params: { path: { id: campaignId } },
      body: { creatorIds },
    },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
  }
  return data;
}

export async function remindCampaignCreatorsSigning(
  campaignId: string,
  creatorIds: string[],
): Promise<CampaignNotifyResult> {
  const { data, error, response } = await client.POST(
    "/campaigns/{id}/remind-signing",
    {
      params: { path: { id: campaignId } },
      body: { creatorIds },
    },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
  }
  return data;
}

export async function patchCampaignCreator(
  campaignId: string,
  creatorId: string,
  patch: CampaignCreatorPatchInput,
): Promise<CampaignCreator> {
  const { data, error, response } = await client.PATCH(
    "/campaigns/{id}/creators/{creatorId}",
    {
      params: { path: { id: campaignId, creatorId } },
      body: patch,
    },
  );
  if (error || !data) {
    const parts = extractErrorParts(error);
    throw new ApiError(
      response?.status ?? 0,
      parts.code,
      parts.message,
      parts.details,
    );
  }
  return data.data;
}
