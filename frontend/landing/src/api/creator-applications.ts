import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CreatorApplicationSubmitRequest =
  components["schemas"]["CreatorApplicationSubmitRequest"];
export type CreatorApplicationSubmitData =
  components["schemas"]["CreatorApplicationSubmitData"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

function extractErrorMessage(error: unknown): string {
  const e = error as { error?: { message?: string } };
  return e?.error?.message ?? "";
}

export async function submitCreatorApplication(
  payload: CreatorApplicationSubmitRequest,
): Promise<CreatorApplicationSubmitData> {
  const { data, error, response } = await client.POST("/creators/applications", {
    body: payload,
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error), extractErrorMessage(error));
  }
  return data.data;
}
