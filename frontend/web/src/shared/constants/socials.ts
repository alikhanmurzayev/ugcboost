import type { components } from "@/api/generated/schema";

export type SocialPlatform = components["schemas"]["SocialPlatform"];

export const PLATFORM_LABELS: Record<SocialPlatform, string> = {
  instagram: "Instagram",
  tiktok: "TikTok",
  threads: "Threads",
};
