import type { components } from "@/api/generated/schema";

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

export const CAMPAIGN_CREATOR_GROUP_ORDER = [
  CAMPAIGN_CREATOR_STATUS.PLANNED,
  CAMPAIGN_CREATOR_STATUS.INVITED,
  CAMPAIGN_CREATOR_STATUS.DECLINED,
  CAMPAIGN_CREATOR_STATUS.AGREED,
  CAMPAIGN_CREATOR_STATUS.SIGNING,
  CAMPAIGN_CREATOR_STATUS.SIGNED,
  CAMPAIGN_CREATOR_STATUS.SIGNING_DECLINED,
] as const satisfies readonly CampaignCreatorStatus[];

// Compile-time exhaustiveness: triggers a TS error if a new status is added
// to OpenAPI but not appended to CAMPAIGN_CREATOR_GROUP_ORDER above.
type MissingStatus = Exclude<
  CampaignCreatorStatus,
  (typeof CAMPAIGN_CREATOR_GROUP_ORDER)[number]
>;
const _orderIsExhaustive: [MissingStatus] extends [never] ? true : never = true;
void _orderIsExhaustive;

export const CREATOR_DRAWER_GROUP_KEYS = {
  ACTIVE: "active",
  IN_PROGRESS: "inProgress",
  REJECTED: "rejected",
} as const;

export type CreatorDrawerGroupKey =
  (typeof CREATOR_DRAWER_GROUP_KEYS)[keyof typeof CREATOR_DRAWER_GROUP_KEYS];

export const CAMPAIGN_CREATOR_DRAWER_GROUPS = [
  {
    groupKey: CREATOR_DRAWER_GROUP_KEYS.ACTIVE,
    statuses: [
      CAMPAIGN_CREATOR_STATUS.SIGNED,
      CAMPAIGN_CREATOR_STATUS.SIGNING,
      CAMPAIGN_CREATOR_STATUS.AGREED,
    ],
  },
  {
    groupKey: CREATOR_DRAWER_GROUP_KEYS.IN_PROGRESS,
    statuses: [
      CAMPAIGN_CREATOR_STATUS.INVITED,
      CAMPAIGN_CREATOR_STATUS.PLANNED,
    ],
  },
  {
    groupKey: CREATOR_DRAWER_GROUP_KEYS.REJECTED,
    statuses: [
      CAMPAIGN_CREATOR_STATUS.DECLINED,
      CAMPAIGN_CREATOR_STATUS.SIGNING_DECLINED,
    ],
  },
] as const satisfies readonly {
  groupKey: CreatorDrawerGroupKey;
  statuses: readonly CampaignCreatorStatus[];
}[];

// Compile-time exhaustiveness: every CampaignCreatorStatus must appear in
// exactly one CAMPAIGN_CREATOR_DRAWER_GROUPS bucket — otherwise a new status
// would slip through the drawer rendering unnoticed.
type DrawerCoveredStatus =
  (typeof CAMPAIGN_CREATOR_DRAWER_GROUPS)[number]["statuses"][number];
type MissingDrawerStatus = Exclude<CampaignCreatorStatus, DrawerCoveredStatus>;
const _drawerGroupsExhaustive: [MissingDrawerStatus] extends [never]
  ? true
  : never = true;
void _drawerGroupsExhaustive;
