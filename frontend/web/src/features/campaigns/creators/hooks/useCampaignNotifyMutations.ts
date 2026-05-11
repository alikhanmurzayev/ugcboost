import { useMutation, type UseMutationResult } from "@tanstack/react-query";
import {
  notifyCampaignCreators,
  remindCampaignCreatorsInvitation,
  remindCampaignCreatorsSigning,
  type CampaignNotifyResult,
} from "@/api/campaignCreators";
import type { ApiError } from "@/api/client";

export interface CampaignNotifyMutations {
  notify: UseMutationResult<CampaignNotifyResult, ApiError, string[]>;
  remind: UseMutationResult<CampaignNotifyResult, ApiError, string[]>;
  remindSigning: UseMutationResult<CampaignNotifyResult, ApiError, string[]>;
}

export function useCampaignNotifyMutations(
  campaignId: string,
): CampaignNotifyMutations {
  // All mutations share onError to satisfy the frontend-api standard
  // (every useMutation must have onError). The actual data-vs-error split
  // is handled per-call in the section's `onSettled`, which has access to
  // both the result envelope and the ApiError.
  const noopOnError = () => {};

  const notify = useMutation<CampaignNotifyResult, ApiError, string[]>({
    mutationFn: (creatorIds) => notifyCampaignCreators(campaignId, creatorIds),
    onError: noopOnError,
  });

  const remind = useMutation<CampaignNotifyResult, ApiError, string[]>({
    mutationFn: (creatorIds) =>
      remindCampaignCreatorsInvitation(campaignId, creatorIds),
    onError: noopOnError,
  });

  const remindSigning = useMutation<CampaignNotifyResult, ApiError, string[]>({
    mutationFn: (creatorIds) =>
      remindCampaignCreatorsSigning(campaignId, creatorIds),
    onError: noopOnError,
  });

  return { notify, remind, remindSigning };
}
