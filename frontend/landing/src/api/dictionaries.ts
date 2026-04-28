import type { components, paths } from "./generated/schema";
import client, { ApiError } from "./client";

export type DictionaryEntry = components["schemas"]["DictionaryEntry"];
// The dictionary type isn't surfaced as a named schema in OpenAPI, so we
// extract it from the path parameter to stay in sync with the contract.
export type DictionaryType =
  paths["/dictionaries/{type}"]["get"]["parameters"]["path"]["type"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

function extractErrorMessage(error: unknown): string {
  const e = error as { error?: { message?: string } };
  return e?.error?.message ?? "";
}

export async function listDictionary(type: DictionaryType): Promise<DictionaryEntry[]> {
  const { data, error, response } = await client.GET("/dictionaries/{type}", {
    params: { path: { type } },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error), extractErrorMessage(error));
  }
  return data.data.items;
}
