export const ROUTES = {
  LOGIN: "/login",
  DASHBOARD: "/",
  BRANDS: "brands",
  BRAND_DETAIL: (id: string) => `brands/${id}`,
  AUDIT: "audit",
} as const;
