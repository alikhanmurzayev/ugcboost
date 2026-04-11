import type { components } from "./generated/schema";
import { api } from "./client";

export type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type AuditLogsResult = components["schemas"]["AuditLogsResult"];

export interface AuditLogsParams {
  actor_id?: string;
  entity_type?: string;
  action?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  per_page?: number;
}

export function listAuditLogs(params: AuditLogsParams = {}) {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      searchParams.set(key, String(value));
    }
  }
  const qs = searchParams.toString();
  return api<AuditLogsResult>(`/audit-logs${qs ? `?${qs}` : ""}`);
}
