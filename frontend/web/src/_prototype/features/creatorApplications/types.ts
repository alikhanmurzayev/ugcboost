import type { CreatorApplicationDetailData as Detail } from "@/_prototype/types/api";

export const ApplicationStages = {
  VERIFICATION: "verification",
  MODERATION: "moderation",
  CONTRACTS: "contracts",
  CREATORS: "creators",
  REJECTED: "rejected",
} as const;

export type ApplicationStage =
  (typeof ApplicationStages)[keyof typeof ApplicationStages];

export const ContractStatuses = {
  NOT_SENT: "not_sent",
  SENT: "sent",
  SIGNED: "signed",
} as const;

export type ContractStatus =
  (typeof ContractStatuses)[keyof typeof ContractStatuses];

export const QualityIndicators = {
  GREEN: "green",
  ORANGE: "orange",
  RED: "red",
} as const;

export type QualityIndicator =
  (typeof QualityIndicators)[keyof typeof QualityIndicators];

export type Application = Omit<Detail, "consents" | "status" | "address"> & {
  stage: ApplicationStage;
  rejectionComment?: string;
  internalNote?: string;
  rejectedAt?: string;
  contractStatus?: ContractStatus;
  approvedAt?: string;
  signedAt?: string;
  // LiveDune-derived auto-quality flag. Wired to a real API later — for now
  // the value is set in the mock so the UI can be designed against it.
  qualityIndicator?: QualityIndicator;
  // Per-account stats pulled from LiveDune (Instagram only on MVP).
  // engagementRate is computed by views (ERV) — see project memory
  // project_livedune_quality_criteria.md for thresholds.
  metrics?: {
    followers: number;
    totalPosts: number;
    avgViews: number;
    engagementRate: number; // %, ERV
    postedLast14Days: boolean;
  };
  // Active-creator stats. Populated for stage='creators' only.
  rating?: number; // 0..5, one decimal place expected
  completedOrders?: number;
  activeOrders?: number;
};

export interface QueueCounts {
  verification: number;
  moderation: number;
  contracts: number;
  creators: number;
  rejected: number;
}
