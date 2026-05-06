import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CreatorListItem = components["schemas"]["CreatorListItem"];
export type CreatorAggregate = components["schemas"]["CreatorAggregate"];
export type CreatorListSortField = components["schemas"]["CreatorListSortField"];
export type CreatorsListInput = components["schemas"]["CreatorsListRequest"];
export type CreatorsListData = components["schemas"]["CreatorsListData"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function listCreators(input: CreatorsListInput) {
  const { data, error, response } = await client.POST("/creators/list", {
    body: input,
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

export async function getCreator(id: string) {
  const { data, error, response } = await client.GET("/creators/{id}", {
    params: { path: { id } },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}
