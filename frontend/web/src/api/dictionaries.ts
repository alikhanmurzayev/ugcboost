import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type DictionaryItem = components["schemas"]["DictionaryItem"];
export type DictionaryType = "categories" | "cities";

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function listDictionary(type: DictionaryType) {
  const { data, error, response } = await client.GET("/dictionaries/{type}", {
    params: { path: { type } },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}
