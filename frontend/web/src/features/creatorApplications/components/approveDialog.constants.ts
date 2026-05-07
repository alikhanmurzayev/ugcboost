// Shared between the dialog component and its unit-test so the
// listCampaigns assertion stays in sync with the actual query payload.
export const CAMPAIGNS_QUERY_PARAMS = {
  page: 1,
  perPage: 100,
  sort: "name" as const,
  order: "asc" as const,
  isDeleted: false,
};
