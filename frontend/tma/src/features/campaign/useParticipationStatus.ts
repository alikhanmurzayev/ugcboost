import { useQuery } from "@tanstack/react-query";

import { apiClient } from "../../api/client";
import type { TmaParticipationResult } from "../../api/types";
import { tmaQueryKeys } from "../../shared/api/queryKeys";
import { secretTokenFormat } from "./campaigns";

// isParticipationResult only checks that `status` is a string. We intentionally
// do NOT reject unknown enum values here — a future backend status the frontend
// hasn't seen should hide the buttons (canDecide compares to INVITED only), not
// throw the query into error and silently block an `invited` creator if some
// other status briefly appears alongside it.
function isParticipationResult(payload: unknown): payload is TmaParticipationResult {
  if (typeof payload !== "object" || payload === null) return false;
  if (!("status" in payload)) return false;
  const status = Reflect.get(payload, "status");
  return typeof status === "string";
}

async function fetchParticipation(
  secretToken: string,
): Promise<TmaParticipationResult> {
  const { data, error } = await apiClient.GET(
    "/tma/campaigns/{secretToken}/participation",
    { params: { path: { secretToken } } },
  );
  if (error) throw error;
  if (!isParticipationResult(data)) {
    throw new Error("invalid participation response");
  }
  return data;
}

export function useParticipationStatus(
  secretToken: string | undefined,
  ndaAccepted: boolean,
) {
  // ndaAccepted gate prevents fingerprint leak (telegram_user_id + secret_token
  // hitting the backend) before the user accepts the privacy policy.
  const enabled =
    ndaAccepted && !!secretToken && secretTokenFormat.test(secretToken);
  return useQuery<TmaParticipationResult>({
    queryKey: tmaQueryKeys.participation(secretToken ?? ""),
    queryFn: async () => {
      if (!secretToken) throw new Error("secretToken missing");
      return fetchParticipation(secretToken);
    },
    enabled,
    retry: false,
  });
}
