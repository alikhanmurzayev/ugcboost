import type { components } from "@/api/generated/schema";

type CampaignCreatorStatus = components["schemas"]["CampaignCreatorStatus"];

export const CAMPAIGN_CREATOR_STATUS = {
  PLANNED: "planned",
  INVITED: "invited",
  DECLINED: "declined",
  AGREED: "agreed",
} as const satisfies Record<string, CampaignCreatorStatus>;

export const CAMPAIGN_CREATOR_GROUP_ORDER = [
  CAMPAIGN_CREATOR_STATUS.PLANNED,
  CAMPAIGN_CREATOR_STATUS.INVITED,
  CAMPAIGN_CREATOR_STATUS.DECLINED,
  CAMPAIGN_CREATOR_STATUS.AGREED,
] as const satisfies readonly CampaignCreatorStatus[];

// Compile-time exhaustiveness: triggers a TS error if a new status is added
// to OpenAPI but not appended to CAMPAIGN_CREATOR_GROUP_ORDER above.
type MissingStatus = Exclude<
  CampaignCreatorStatus,
  (typeof CAMPAIGN_CREATOR_GROUP_ORDER)[number]
>;
const _orderIsExhaustive: [MissingStatus] extends [never] ? true : never = true;
void _orderIsExhaustive;
