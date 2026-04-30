import { MOCK_CAMPAIGNS } from "@/features/campaigns/_mock/campaigns";
import { MOCK_APPLICATIONS } from "@/features/campaigns/_mock/campaignApplications";
import type {
  ApplicationStatus,
  Campaign,
  CampaignApplication,
  CampaignStatus,
  TzStatus,
} from "@/features/campaigns/types";

const MOCK_LATENCY_MS = 200;

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), MOCK_LATENCY_MS));
}

export async function listCampaigns(status?: CampaignStatus): Promise<Campaign[]> {
  if (status) {
    return delay(MOCK_CAMPAIGNS.filter((c) => c.status === status));
  }
  return delay([...MOCK_CAMPAIGNS]);
}

export async function getCampaign(id: string): Promise<Campaign | undefined> {
  return delay(MOCK_CAMPAIGNS.find((c) => c.id === id));
}

export type CampaignCounts = Record<CampaignStatus, number>;

export async function getCampaignCounts(): Promise<CampaignCounts> {
  const counts: CampaignCounts = {
    draft: 0,
    pending_moderation: 0,
    rejected: 0,
    active: 0,
    completed: 0,
  };
  for (const c of MOCK_CAMPAIGNS) counts[c.status] += 1;
  return delay(counts);
}

export type CampaignDraftInput = Omit<
  Campaign,
  "id" | "brandId" | "brandName" | "status" | "createdAt" | "updatedAt" | "publishedAt"
>;

export async function createCampaign(
  input: CampaignDraftInput,
  asDraft: boolean,
): Promise<Campaign> {
  const now = new Date().toISOString();
  const id = `c1000000-0000-0000-0000-${Math.random().toString(16).slice(2, 14).padStart(12, "0")}`;
  const created: Campaign = {
    ...input,
    id,
    brandId: "00000000-0000-0000-0000-0000000000aa",
    brandName: "Fixprice",
    status: asDraft ? "draft" : "pending_moderation",
    createdAt: now,
    updatedAt: now,
  };
  MOCK_CAMPAIGNS.unshift(created);
  return delay(created);
}

export async function listCampaignApplications(
  campaignId: string,
): Promise<CampaignApplication[]> {
  const list = MOCK_APPLICATIONS.filter((a) => a.campaignId === campaignId);
  return delay(list.map((a) => ({ ...a })));
}

export async function setApplicationStatus(
  applicationId: string,
  status: ApplicationStatus,
): Promise<CampaignApplication> {
  const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === applicationId);
  if (idx < 0) throw new Error("not_found");
  const current = MOCK_APPLICATIONS[idx]!;
  const next: CampaignApplication = {
    ...current,
    status,
    decidedAt: new Date().toISOString(),
  };
  MOCK_APPLICATIONS[idx] = next;
  return delay(next);
}

// Brand-level "Подписать ТЗ с креаторами" — flips every approved application
// in the campaign whose tzStatus is still "not_sent" to "sent".
export async function sendTzToApproved(campaignId: string): Promise<number> {
  const now = new Date().toISOString();
  let count = 0;
  for (let i = 0; i < MOCK_APPLICATIONS.length; i++) {
    const a = MOCK_APPLICATIONS[i]!;
    if (
      a.campaignId === campaignId &&
      a.status === "approved" &&
      a.tzStatus === "not_sent"
    ) {
      MOCK_APPLICATIONS[i] = { ...a, tzStatus: "sent", tzSentAt: now };
      count += 1;
    }
  }
  return delay(count);
}

export async function setTzStatus(
  applicationId: string,
  tzStatus: TzStatus,
): Promise<CampaignApplication> {
  const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === applicationId);
  if (idx < 0) throw new Error("not_found");
  const current = MOCK_APPLICATIONS[idx]!;
  const next: CampaignApplication = {
    ...current,
    tzStatus,
    tzDecidedAt:
      tzStatus === "accepted" || tzStatus === "declined"
        ? new Date().toISOString()
        : current.tzDecidedAt,
  };
  MOCK_APPLICATIONS[idx] = next;
  return delay(next);
}

// Replacement: marks the declined creator as "replaced" (hidden) and approves
// the chosen pending application, immediately flipping it to tzStatus "sent".
export async function replaceCreator(
  declinedApplicationId: string,
  replacementApplicationId: string,
): Promise<{ replaced: CampaignApplication; replacement: CampaignApplication }> {
  const declinedIdx = MOCK_APPLICATIONS.findIndex(
    (a) => a.id === declinedApplicationId,
  );
  const replacementIdx = MOCK_APPLICATIONS.findIndex(
    (a) => a.id === replacementApplicationId,
  );
  if (declinedIdx < 0 || replacementIdx < 0) throw new Error("not_found");
  const now = new Date().toISOString();
  const replaced: CampaignApplication = {
    ...MOCK_APPLICATIONS[declinedIdx]!,
    tzStatus: "replaced",
  };
  const replacement: CampaignApplication = {
    ...MOCK_APPLICATIONS[replacementIdx]!,
    status: "approved",
    decidedAt: now,
    tzStatus: "sent",
    tzSentAt: now,
  };
  MOCK_APPLICATIONS[declinedIdx] = replaced;
  MOCK_APPLICATIONS[replacementIdx] = replacement;
  return delay({ replaced, replacement });
}

export async function updateCampaign(
  id: string,
  input: CampaignDraftInput,
  asDraft: boolean,
): Promise<Campaign> {
  const idx = MOCK_CAMPAIGNS.findIndex((c) => c.id === id);
  if (idx < 0) throw new Error("not_found");
  const current = MOCK_CAMPAIGNS[idx]!;
  const next: Campaign = {
    ...current,
    ...input,
    id: current.id,
    brandId: current.brandId,
    brandName: current.brandName,
    status: asDraft ? "draft" : "pending_moderation",
    createdAt: current.createdAt,
    updatedAt: new Date().toISOString(),
  };
  MOCK_CAMPAIGNS[idx] = next;
  return delay(next);
}
