// Shared between the dialog component and its unit-test so the
// listCampaigns assertion stays in sync with the actual query payload.
//
// `created_at desc` + `perPage = backend max (200)` keeps the dialog usable
// when the campaign roster grows: newest-first matches admin intent (a fresh
// approval typically targets a recently-created campaign), and the cap pulls
// the full backend page so a campaign that sorts late by name (e.g. starts
// with `z` or a UUID-style prefix) is still reachable. Server-side search
// would scale better past 200 — captured as a follow-up; a 200-entry roster
// is the assumed ceiling for now.
export const CAMPAIGNS_QUERY_PARAMS = {
  page: 1,
  perPage: 200,
  sort: "created_at" as const,
  order: "desc" as const,
  isDeleted: false,
};
