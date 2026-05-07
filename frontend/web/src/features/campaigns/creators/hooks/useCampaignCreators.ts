import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listCampaignCreators } from "@/api/campaignCreators";
import type { CampaignCreator } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem, CreatorsListInput } from "@/api/creators";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";

export interface CampaignCreatorRow {
  campaignCreator: CampaignCreator;
  creator?: CreatorListItem;
}

export interface UseCampaignCreatorsResult {
  rows: CampaignCreatorRow[];
  total: number;
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
}

const PER_PAGE = 200;

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

  const profilesInput: CreatorsListInput = {
    ids: creatorIds,
    page: 1,
    perPage: PER_PAGE,
    sort: "created_at",
    order: "desc",
  };

  const profilesQuery = useQuery({
    queryKey: campaignCreatorKeys.profiles(campaignId, creatorIds),
    queryFn: () => listCreators(profilesInput),
    enabled: profilesEnabled,
  });

  const rows = useMemo<CampaignCreatorRow[]>(() => {
    const ccs = ccQuery.data ?? [];
    if (ccs.length === 0) return [];
    const profiles = profilesQuery.data?.data?.items ?? [];
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

  const total = ccQuery.data?.length ?? 0;
  const isLoading =
    ccQuery.isLoading || (profilesEnabled && profilesQuery.isLoading);
  const isError = ccQuery.isError || (profilesEnabled && profilesQuery.isError);

  function refetch() {
    void ccQuery.refetch();
    if (profilesEnabled) void profilesQuery.refetch();
  }

  return { rows, total, isLoading, isError, refetch };
}
