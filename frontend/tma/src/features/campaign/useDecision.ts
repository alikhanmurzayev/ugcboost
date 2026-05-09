import { useMutation } from "@tanstack/react-query";

import { apiClient } from "../../api/client";
import type { APIError, TmaDecisionResult } from "../../api/types";
import { CAMPAIGN_CREATOR_STATUS } from "../../shared/constants/campaignCreatorStatus";

export type DecisionKind = "agree" | "decline";

export type DecisionError = APIError & { status: number };

export type DecisionResult = TmaDecisionResult;

const NETWORK_ERROR_CODE = "NETWORK_ERROR";
const INTERNAL_ERROR_CODE = "INTERNAL_ERROR";

function isDecisionResult(payload: unknown): payload is DecisionResult {
  if (typeof payload !== "object" || payload === null) return false;
  const obj = payload as Record<string, unknown>;
  return (
    (obj.status === CAMPAIGN_CREATOR_STATUS.AGREED ||
      obj.status === CAMPAIGN_CREATOR_STATUS.DECLINED) &&
    typeof obj.alreadyDecided === "boolean"
  );
}

function extractApiError(payload: unknown): { code: string; message: string } {
  const wrapper =
    typeof payload === "object" && payload !== null
      ? (payload as { error?: unknown }).error
      : undefined;
  const inner =
    typeof wrapper === "object" && wrapper !== null
      ? (wrapper as { code?: unknown; message?: unknown })
      : undefined;
  return {
    code: typeof inner?.code === "string" ? inner.code : INTERNAL_ERROR_CODE,
    message: typeof inner?.message === "string" ? inner.message : "",
  };
}

async function callDecision(
  kind: DecisionKind,
  secretToken: string,
): Promise<DecisionResult> {
  const path =
    kind === "agree"
      ? "/tma/campaigns/{secretToken}/agree"
      : "/tma/campaigns/{secretToken}/decline";
  let result: Awaited<ReturnType<typeof apiClient.POST>>;
  try {
    result = await apiClient.POST(path, { params: { path: { secretToken } } });
  } catch {
    throw {
      status: 0,
      code: NETWORK_ERROR_CODE,
      message: "",
    } as DecisionError;
  }
  const { data, error, response } = result;
  const status = response?.status ?? 0;
  if (error) {
    const { code, message } = extractApiError(error);
    throw { status, code, message } as DecisionError;
  }
  if (!isDecisionResult(data)) {
    throw {
      status,
      code: INTERNAL_ERROR_CODE,
      message: "",
    } as DecisionError;
  }
  return data;
}

// onError sink — the mutation's `error` is rendered inline via
// CampaignBriefPage (`agree.error ?? decline.error`). The handler here keeps
// the standard's "useMutation must have onError" rule explicit; surfacing the
// payload to the UI happens through the returned mutation, not a side effect.
function onDecisionError(_error: DecisionError): void {
  // intentionally empty
}

export function useAgreeDecision(secretToken: string | undefined) {
  return useMutation<DecisionResult, DecisionError, void>({
    mutationFn: () => {
      if (!secretToken) {
        return Promise.reject({
          status: 0,
          code: INTERNAL_ERROR_CODE,
          message: "secretToken missing",
        } as DecisionError);
      }
      return callDecision("agree", secretToken);
    },
    onError: onDecisionError,
  });
}

export function useDeclineDecision(secretToken: string | undefined) {
  return useMutation<DecisionResult, DecisionError, void>({
    mutationFn: () => {
      if (!secretToken) {
        return Promise.reject({
          status: 0,
          code: INTERNAL_ERROR_CODE,
          message: "secretToken missing",
        } as DecisionError);
      }
      return callDecision("decline", secretToken);
    },
    onError: onDecisionError,
  });
}
