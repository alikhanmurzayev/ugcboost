import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type AuditLogEntry = components["schemas"]["AuditLogEntry"];

type ErrorBody = components["schemas"]["ErrorResponse"];

export async function listAuditLogs(params: {
  actor_id?: string;
  entity_type?: string;
  action?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  per_page?: number;
} = {}) {
  const { data, error, response } = await client.GET("/audit-logs", {
    params: { query: params },
  });
  if (error) {
    const e = error as unknown as ErrorBody;
    throw new ApiError(response.status, e.error?.code ?? "INTERNAL_ERROR");
  }
  return data;
}
