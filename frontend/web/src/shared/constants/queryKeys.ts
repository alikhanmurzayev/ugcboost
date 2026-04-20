export const brandKeys = {
  all: () => ["brands"] as const,
  detail: (id: string) => ["brands", id] as const,
};

export const auditKeys = {
  list: (filters: Record<string, unknown>) => ["audit-logs", filters] as const,
};
