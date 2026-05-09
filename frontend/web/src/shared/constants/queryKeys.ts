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

export const creatorKeys = {
  all: () => ["creators"] as const,
  list: (params: Record<string, unknown>) => ["creators", "list", params] as const,
  detail: (id: string) => ["creators", "detail", id] as const,
};

export const campaignKeys = {
  all: () => ["campaigns"] as const,
  lists: () => ["campaigns", "list"] as const,
  list: (params: Record<string, unknown>) =>
    ["campaigns", "list", params] as const,
  detail: (id: string) => ["campaigns", "detail", id] as const,
  contractTemplate: (id: string) =>
    ["campaigns", id, "contractTemplate"] as const,
};

export const campaignCreatorKeys = {
  all: () => ["campaignCreators"] as const,
  list: (campaignId: string) =>
    ["campaignCreators", "list", campaignId] as const,
  profiles: (campaignId: string, ids: readonly string[]) =>
    ["campaignCreators", "profiles", campaignId, ids] as const,
};

export const dictionaryKeys = {
  list: (type: string) => ["dictionaries", type] as const,
};
