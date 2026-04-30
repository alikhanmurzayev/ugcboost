// Query keys for the prototype area. Kept separate from main src/shared/constants/queryKeys.ts
// so the prototype doesn't pollute the real app namespace. All keys are prefixed with
// "prototype" to guarantee no collision with future real query keys.
export const creatorApplicationKeys = {
  all: () => ["prototype", "creator-applications"] as const,
  list: (stage: string, filters?: Record<string, unknown>) =>
    ["prototype", "creator-applications", "list", stage, filters ?? {}] as const,
  counts: () => ["prototype", "creator-applications", "counts"] as const,
  detail: (id: string) => ["prototype", "creator-applications", "detail", id] as const,
};

export const dictionaryKeys = {
  list: (type: string) => ["prototype", "dictionaries", type] as const,
};

export const campaignKeys = {
  all: () => ["prototype", "campaigns"] as const,
  list: (status?: string) => ["prototype", "campaigns", "list", status ?? "all"] as const,
  counts: () => ["prototype", "campaigns", "counts"] as const,
  detail: (id: string) => ["prototype", "campaigns", "detail", id] as const,
  applications: (campaignId: string) =>
    ["prototype", "campaigns", "applications", campaignId] as const,
};
