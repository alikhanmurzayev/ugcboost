import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listCampaignCreators } from "@/api/campaignCreators";
import type { CampaignCreator } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem } from "@/api/creators";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";

export interface CampaignCreatorRow {
  campaignCreator: CampaignCreator;
  creator?: CreatorListItem;
}

export interface UseCampaignCreatorsResult {
  rows: CampaignCreatorRow[];
  total: number;
  existingCreatorIds: Set<string>;
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
}

// Backend caps `ids` at 200 per /creators/list call (CreatorListIDsMax).
// A campaign with 200+ members fans out into parallel chunked fetches.
const IDS_PER_CHUNK = 200;

async function fetchProfilesByIds(
  ids: readonly string[],
): Promise<CreatorListItem[]> {
  if (ids.length === 0) return [];
  const chunks: string[][] = [];
  for (let i = 0; i < ids.length; i += IDS_PER_CHUNK) {
    chunks.push(ids.slice(i, i + IDS_PER_CHUNK));
  }
  const responses = await Promise.all(
    chunks.map((chunk) =>
      listCreators({
        ids: chunk,
        page: 1,
        perPage: chunk.length,
        sort: "created_at",
        order: "desc",
      }),
    ),
  );
  return responses.flatMap((r) => r.data?.items ?? []);
}

export function useCampaignCreators(
  campaignId: string,
  options: { enabled?: boolean } = {},
): UseCampaignCreatorsResult {
  const enabled = options.enabled ?? true;

  const ccQuery = useQuery({
    queryKey: campaignCreatorKeys.list(campaignId),
    queryFn: () => listCampaignCreators(campaignId),
    enabled: enabled && !!campaignId,
  });

  const creatorIds = useMemo(() => {
    const ids = (ccQuery.data ?? []).map((cc) => cc.creatorId);
    return [...ids].sort();
  }, [ccQuery.data]);

  const profilesEnabled = enabled && creatorIds.length > 0;

  const profilesQuery = useQuery({
    queryKey: campaignCreatorKeys.profiles(campaignId, creatorIds),
    queryFn: () => fetchProfilesByIds(creatorIds),
    enabled: profilesEnabled,
  });

  const rows = useMemo<CampaignCreatorRow[]>(() => {
    const ccs = ccQuery.data ?? [];
    if (ccs.length === 0) return [];
    const profiles = profilesQuery.data ?? [];
    const ccByCreatorId = new Map<string, CampaignCreator>(
      ccs.map((cc) => [cc.creatorId, cc]),
    );

    const known: CampaignCreatorRow[] = [];
    const seen = new Set<string>();
    for (const profile of profiles) {
      const cc = ccByCreatorId.get(profile.id);
      if (!cc) continue;
      seen.add(profile.id);
      known.push({ campaignCreator: cc, creator: profile });
    }
    const missing: CampaignCreatorRow[] = ccs
      .filter((cc) => !seen.has(cc.creatorId))
      .map((cc) => ({ campaignCreator: cc }));
    return [...known, ...missing];
  }, [ccQuery.data, profilesQuery.data]);

  const existingCreatorIds = useMemo(() => {
    return new Set((ccQuery.data ?? []).map((cc) => cc.creatorId));
  }, [ccQuery.data]);

  const total = ccQuery.data?.length ?? 0;
  const isLoading =
    ccQuery.isLoading || (profilesEnabled && profilesQuery.isLoading);
  const isError = ccQuery.isError || (profilesEnabled && profilesQuery.isError);

  function refetch() {
    void ccQuery.refetch();
    if (profilesEnabled) void profilesQuery.refetch();
  }

  return { rows, total, existingCreatorIds, isLoading, isError, refetch };
}
