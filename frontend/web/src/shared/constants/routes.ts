export const ROUTES = {
  LOGIN: "/login",
  DASHBOARD: "/",
  BRANDS: "brands",
  BRAND_DETAIL: (id: string) => `brands/${id}`,
  BRAND_DETAIL_PATTERN: "brands/:brandId",
  AUDIT: "audit",
  CREATOR_APP_VERIFICATION: "creator-applications/verification",
  CREATOR_APP_MODERATION: "creator-applications/moderation",
  CREATOR_APP_REJECTED: "creator-applications/rejected",
  CREATORS: "creators",
  CAMPAIGNS: "campaigns",
  CAMPAIGN_NEW: "campaigns/new",
  CAMPAIGN_DETAIL: (id: string) => `campaigns/${id}`,
  CAMPAIGN_DETAIL_PATTERN: "campaigns/:campaignId",
} as const;

export const SEARCH_PARAMS = {
  CREATOR_ID: "creatorId",
} as const;
