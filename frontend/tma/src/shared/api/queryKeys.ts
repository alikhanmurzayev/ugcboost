// tmaQueryKeys is the hierarchical factory for React Query keys in TMA.
// Per `docs/standards/frontend-api.md`, string-literal query keys in
// components are forbidden — every key flows through this factory so the
// hierarchy enables cache invalidation of a parent without listing children.
export const tmaQueryKeys = {
  participation: (secretToken: string) =>
    ["tma", "participation", secretToken] as const,
};
