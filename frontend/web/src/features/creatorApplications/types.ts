import type { components } from "@/api/generated/schema";

export type Application = components["schemas"]["CreatorApplicationListItem"];
export type ApplicationDetail = components["schemas"]["CreatorApplicationDetailData"];
export type ApplicationStatus = components["schemas"]["CreatorApplicationStatus"];
export type SocialPlatform = components["schemas"]["SocialPlatform"];
export type DictionaryItem = components["schemas"]["DictionaryItem"];

export const PLATFORM_LABELS: Record<SocialPlatform, string> = {
  instagram: "Instagram",
  tiktok: "TikTok",
  threads: "Threads",
};
