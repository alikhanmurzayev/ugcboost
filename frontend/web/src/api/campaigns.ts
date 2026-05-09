import type { components, paths } from "./generated/schema";
import client, { ApiError } from "./client";

export type Campaign = components["schemas"]["Campaign"];
export type CampaignListSortField = components["schemas"]["CampaignListSortField"];
export type CampaignsListData = components["schemas"]["CampaignsListData"];
export type CampaignsListInput = paths["/campaigns"]["get"]["parameters"]["query"];
export type CampaignInput = components["schemas"]["CampaignInput"];
export type CampaignCreatedResult = components["schemas"]["CampaignCreatedResult"];
export type GetCampaignResult = components["schemas"]["GetCampaignResult"];
export type UploadCampaignContractTemplateResult =
  components["schemas"]["UploadCampaignContractTemplateResult"];
export type ContractValidationDetails =
  components["schemas"]["ContractValidationDetails"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

function extractErrorMessage(error: unknown): string | undefined {
  const e = error as { error?: { message?: string } };
  return e?.error?.message;
}

function extractErrorDetails(error: unknown): unknown {
  const e = error as { error?: { details?: unknown } };
  return e?.error?.details;
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

export async function getCampaign(id: string): Promise<GetCampaignResult> {
  const { data, error, response } = await client.GET("/campaigns/{id}", {
    params: { path: { id } },
  });
  if (error || !data) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function updateCampaign(
  id: string,
  input: CampaignInput,
): Promise<void> {
  const { error, response } = await client.PATCH("/campaigns/{id}", {
    params: { path: { id } },
    body: input,
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
}

export async function uploadCampaignContractTemplate(
  campaignId: string,
  pdf: Blob,
): Promise<UploadCampaignContractTemplateResult> {
  const { data, error, response } = await client.PUT(
    "/campaigns/{id}/contract-template",
    {
      params: { path: { id: campaignId } },
      body: pdf as unknown as never,
      bodySerializer: (b) => b as BodyInit,
      headers: { "Content-Type": "application/pdf" },
      parseAs: "json",
    },
  );
  if (error || !data) {
    throw new ApiError(
      response.status,
      extractErrorCode(error),
      extractErrorMessage(error),
      extractErrorDetails(error),
    );
  }
  return data;
}

export async function downloadCampaignContractTemplate(
  campaignId: string,
): Promise<Blob> {
  const { data, error, response } = await client.GET(
    "/campaigns/{id}/contract-template",
    {
      params: { path: { id: campaignId } },
      parseAs: "blob",
    },
  );
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data as unknown as Blob;
}
