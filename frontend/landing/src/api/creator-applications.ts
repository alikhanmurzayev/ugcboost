import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type CreatorApplicationSubmitRequest =
  components["schemas"]["CreatorApplicationSubmitRequest"];
export type CreatorApplicationSubmitData =
  components["schemas"]["CreatorApplicationSubmitData"];

export async function submitCreatorApplication(
  payload: CreatorApplicationSubmitRequest,
): Promise<CreatorApplicationSubmitData> {
  const { data, error, response } = await client.POST("/creators/applications", {
    body: payload,
  });
  if (error) {
    throw ApiError.fromResponse(response.status, error);
  }
  return data.data;
}
