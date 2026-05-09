import { useMutation } from "@tanstack/react-query";

import { apiClient } from "../../api/client";

export type DecisionKind = "agree" | "decline";

export type DecisionError = {
  status: number;
  code: string;
  message: string;
};

export type DecisionResult = {
  status: "agreed" | "declined";
  alreadyDecided: boolean;
};

function isDecisionResult(payload: unknown): payload is DecisionResult {
  if (typeof payload !== "object" || payload === null) return false;
  const obj = payload as Record<string, unknown>;
  return (
    (obj.status === "agreed" || obj.status === "declined") &&
    typeof obj.alreadyDecided === "boolean"
  );
}

async function callDecision(
  kind: DecisionKind,
  secretToken: string,
): Promise<DecisionResult> {
  const path =
    kind === "agree"
      ? "/tma/campaigns/{secretToken}/agree"
      : "/tma/campaigns/{secretToken}/decline";
  const { data, error, response } = await apiClient.POST(path, {
    params: { path: { secretToken } },
  });
  if (error) {
    const code =
      typeof (error as { error?: { code?: unknown } }).error?.code === "string"
        ? ((error as { error: { code: string } }).error.code)
        : "INTERNAL_ERROR";
    const message =
      typeof (error as { error?: { message?: unknown } }).error?.message ===
      "string"
        ? ((error as { error: { message: string } }).error.message)
        : "";
    throw { status: response.status, code, message } as DecisionError;
  }
  if (!isDecisionResult(data)) {
    throw {
      status: response.status,
      code: "INTERNAL_ERROR",
      message: "",
    } as DecisionError;
  }
  return data;
}

export function useAgreeDecision(secretToken: string | undefined) {
  return useMutation<DecisionResult, DecisionError, void>({
    mutationFn: () => {
      if (!secretToken) {
        return Promise.reject({
          status: 0,
          code: "INTERNAL_ERROR",
          message: "secretToken missing",
        } as DecisionError);
      }
      return callDecision("agree", secretToken);
    },
  });
}

export function useDeclineDecision(secretToken: string | undefined) {
  return useMutation<DecisionResult, DecisionError, void>({
    mutationFn: () => {
      if (!secretToken) {
        return Promise.reject({
          status: 0,
          code: "INTERNAL_ERROR",
          message: "secretToken missing",
        } as DecisionError);
      }
      return callDecision("decline", secretToken);
    },
  });
}
