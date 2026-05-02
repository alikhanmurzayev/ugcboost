import type { components, paths } from "./generated/schema";
import client, { ApiError } from "./client";

export type DictionaryItem = components["schemas"]["DictionaryItem"];
// The dictionary type isn't surfaced as a named schema in OpenAPI, so we
// extract it from the path parameter to stay in sync with the contract.
export type DictionaryType =
  paths["/dictionaries/{type}"]["get"]["parameters"]["path"]["type"];

export async function listDictionary(type: DictionaryType): Promise<DictionaryItem[]> {
  const { data, error, response } = await client.GET("/dictionaries/{type}", {
    params: { path: { type } },
  });
  if (error) {
    throw ApiError.fromResponse(response.status, error);
  }
  return data.data.items;
}
