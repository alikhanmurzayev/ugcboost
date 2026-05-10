import type { components } from "../../api/generated/schema";

type CampaignCreatorStatus = components["schemas"]["CampaignCreatorStatus"];

export const CAMPAIGN_CREATOR_STATUS = {
  PLANNED: "planned",
  INVITED: "invited",
  DECLINED: "declined",
  AGREED: "agreed",
  SIGNING: "signing",
  SIGNED: "signed",
  SIGNING_DECLINED: "signing_declined",
} as const satisfies Record<string, CampaignCreatorStatus>;

type MissingStatus = Exclude<
  CampaignCreatorStatus,
  (typeof CAMPAIGN_CREATOR_STATUS)[keyof typeof CAMPAIGN_CREATOR_STATUS]
>;
const _statusIsExhaustive: [MissingStatus] extends [never] ? true : never = true;
void _statusIsExhaustive;
