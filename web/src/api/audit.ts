import { api } from "./client";

export interface AuditLogEntry {
  id: string;
  actorId: string;
  actorRole: string;
  action: string;
  entityType: string;
  entityId?: string;
  oldValue?: unknown;
  newValue?: unknown;
  ipAddress: string;
  createdAt: string;
}

interface AuditLogsResponse {
  data: {
    logs: AuditLogEntry[];
    total: number;
    page: number;
    perPage: number;
  };
}

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
  return api<AuditLogsResponse>(`/audit-logs${qs ? `?${qs}` : ""}`);
}
