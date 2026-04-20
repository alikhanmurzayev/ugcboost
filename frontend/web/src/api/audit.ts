import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type AuditLogEntry = components["schemas"]["AuditLogEntry"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

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
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}
