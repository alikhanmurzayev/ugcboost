export const ROUTES = {
  LOGIN: "/login",
  DASHBOARD: "/",
  BRANDS: "brands",
  BRAND_DETAIL: (id: string) => `brands/${id}`,
  BRAND_DETAIL_PATTERN: "brands/:brandId",
  AUDIT: "audit",
} as const;
