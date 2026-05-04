import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CreatorApplicationListItem = components["schemas"]["CreatorApplicationListItem"];
export type CreatorApplicationDetail = components["schemas"]["CreatorApplicationDetailData"];
export type CreatorApplicationStatus = components["schemas"]["CreatorApplicationStatus"];
export type CreatorApplicationListSortField = components["schemas"]["CreatorApplicationListSortField"];
export type SortOrder = components["schemas"]["SortOrder"];
export type CreatorApplicationsListInput = components["schemas"]["CreatorApplicationsListRequest"];
export type CreatorApplicationsListData = components["schemas"]["CreatorApplicationsListData"];
export type CreatorApplicationStatusCount = components["schemas"]["CreatorApplicationStatusCount"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function listCreatorApplications(input: CreatorApplicationsListInput) {
  const { data, error, response } = await client.POST("/creators/applications/list", {
    body: input,
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function getCreatorApplication(id: string) {
  const { data, error, response } = await client.GET("/creators/applications/{id}", {
    params: { path: { id } },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function getCreatorApplicationsCounts() {
  const { data, error, response } = await client.GET("/creators/applications/counts");
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function verifyApplicationSocialManually(applicationId: string, socialId: string) {
  const { data, error, response } = await client.POST(
    "/creators/applications/{id}/socials/{socialId}/verify",
    {
      params: { path: { id: applicationId, socialId } },
      body: {},
    },
  );
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}
