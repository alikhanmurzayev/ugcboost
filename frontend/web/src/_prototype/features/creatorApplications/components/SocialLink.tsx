import type { components } from "@/api/generated/schema";

type SocialPlatform = components["schemas"]["SocialPlatform"];

const PLATFORM_LABELS: Record<SocialPlatform, string> = {
  instagram: "Instagram",
  tiktok: "TikTok",
  threads: "Threads",
};

const PLATFORM_ICON_SRC: Record<SocialPlatform, string> = {
  instagram: "/social/instagram.svg",
  tiktok: "/social/tiktok.svg",
  threads: "/social/threads.svg",
};

function buildUrl(platform: SocialPlatform, handle: string): string {
  const clean = handle.replace(/^@/, "");
  switch (platform) {
    case "instagram":
      return `https://instagram.com/${clean}`;
    case "tiktok":
      return `https://tiktok.com/@${clean}`;
    case "threads":
      return `https://threads.net/@${clean}`;
  }
}

interface SocialLinkProps {
  platform: SocialPlatform;
  handle: string;
  showHandle?: boolean;
}

export default function SocialLink({
  platform,
  handle,
  showHandle = false,
}: SocialLinkProps) {
  const url = buildUrl(platform, handle);
  const label = PLATFORM_LABELS[platform];
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`${label} ${handle}`}
      className="inline-flex items-center gap-1.5 text-gray-700 transition hover:opacity-70"
      data-testid={`social-${platform}`}
    >
      <img
        src={PLATFORM_ICON_SRC[platform]}
        alt=""
        className="h-4 w-4 shrink-0"
        aria-hidden="true"
      />
      {showHandle && <span className="text-sm">@{handle}</span>}
    </a>
  );
}
