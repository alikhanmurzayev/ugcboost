export const brandKeys = {
  all: () => ["brands"] as const,
  detail: (id: string) => ["brands", id] as const,
};

export const auditKeys = {
  list: (filters: Record<string, unknown>) => ["audit-logs", filters] as const,
};

export const creatorApplicationKeys = {
  all: () => ["creator-applications"] as const,
  list: (params: Record<string, unknown>) =>
    ["creator-applications", "list", params] as const,
  detail: (id: string) => ["creator-applications", "detail", id] as const,
  counts: () => ["creator-applications", "counts"] as const,
};

export const dictionaryKeys = {
  list: (type: string) => ["dictionaries", type] as const,
};
