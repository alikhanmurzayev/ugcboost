import { ApiError } from "@/api/client";
import type {
  CampaignCreatorBatchInvalidDetail,
  CampaignCreatorStatus,
  CampaignNotifyResult,
  CampaignNotifyUndelivered,
  CampaignNotifyUndeliveredReason,
} from "@/api/campaignCreators";

export const MAX_NOTIFY_BATCH = 200;

export type SectionResultKind =
  | "success"
  | "validation_error"
  | "validation_unknown"
  | "network_error";

export interface SectionResult {
  kind: SectionResultKind;
  undelivered?: CampaignNotifyUndelivered[];
  deliveredCount?: number;
  undeliveredNames?: Record<string, string>;
  validationDetails?: ValidationDetailItem[];
  detailNames?: Record<string, string>;
}

export interface ValidationDetailItem {
  creatorId: string;
  currentStatus: CampaignCreatorStatus;
}

export const UNDELIVERED_REASON_KEY: Record<
  CampaignNotifyUndeliveredReason,
  string
> = {
  bot_blocked: "campaignCreators.undeliveredReason.bot_blocked",
  unknown: "campaignCreators.undeliveredReason.unknown",
};

export function parseSettled(
  data: CampaignNotifyResult | undefined,
  error: unknown,
  attempted: number,
  namesSnapshot: Record<string, string>,
): SectionResult {
  if (error instanceof ApiError && error.status === 422) {
    if (error.code === "CAMPAIGN_CREATOR_BATCH_INVALID") {
      const raw = isBatchInvalidDetails(error.details) ? error.details : [];
      const validationDetails: ValidationDetailItem[] = [];
      for (const d of raw) {
        if (!d.currentStatus) continue;
        validationDetails.push({
          creatorId: d.creatorId,
          currentStatus: d.currentStatus,
        });
      }
      return {
        kind: "validation_error",
        validationDetails,
        detailNames: namesSnapshot,
      };
    }
    return { kind: "validation_unknown" };
  }
  if (error) {
    return { kind: "network_error" };
  }
  if (data) {
    const undelivered = data.data.undelivered;
    return {
      kind: "success",
      undelivered,
      deliveredCount: Math.max(0, attempted - undelivered.length),
      undeliveredNames: namesSnapshot,
    };
  }
  return { kind: "network_error" };
}

export function isBatchInvalidDetails(
  value: unknown,
): value is CampaignCreatorBatchInvalidDetail[] {
  if (!Array.isArray(value)) return false;
  for (const item of value) {
    if (typeof item !== "object" || item === null) return false;
    const candidate = item as {
      creatorId?: unknown;
      currentStatus?: unknown;
    };
    if (typeof candidate.creatorId !== "string") return false;
    if (typeof candidate.currentStatus !== "string") return false;
  }
  return true;
}
