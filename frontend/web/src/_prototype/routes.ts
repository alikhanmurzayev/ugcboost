// Routes for the prototype area (mounted under /prototype/* in main App.tsx).
// All paths are relative — React Router handles the /prototype prefix.
export const ROUTES = {
  CREATOR_APP_VERIFICATION: "creator-applications/verification",
  CREATOR_APP_MODERATION: "creator-applications/moderation",
  CREATOR_APP_CONTRACTS: "creator-applications/contracts",
  CREATOR_APP_REJECTED: "creator-applications/rejected",
  CREATORS: "creators",
  CAMPAIGNS: "campaigns",
  CAMPAIGN_NEW: "campaigns/new",
  CAMPAIGNS_ACTIVE: "campaigns/active",
  CAMPAIGNS_PENDING: "campaigns/pending",
  CAMPAIGNS_REJECTED: "campaigns/rejected",
  CAMPAIGNS_DRAFT: "campaigns/draft",
  CAMPAIGNS_COMPLETED: "campaigns/completed",
  CAMPAIGN_DETAIL: (id: string) => `campaigns/${id}`,
  CAMPAIGN_DETAIL_PATTERN: "campaigns/:campaignId",
  CAMPAIGN_EDIT: (id: string) => `campaigns/${id}/edit`,
  CAMPAIGN_EDIT_PATTERN: "campaigns/:campaignId/edit",
} as const;
