import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import {
  patchCampaignCreator,
  type CampaignCreator,
  type CampaignCreatorPatchInput,
} from "@/api/campaignCreators";
import type { ApiError } from "@/api/client";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";

export interface PatchCampaignCreatorVariables {
  creatorId: string;
  patch: CampaignCreatorPatchInput;
}

interface RollbackContext {
  previous: CampaignCreator[] | undefined;
}

export type UsePatchCampaignCreatorResult = UseMutationResult<
  CampaignCreator,
  ApiError,
  PatchCampaignCreatorVariables,
  RollbackContext
>;

export function usePatchCampaignCreator(
  campaignId: string,
  options: { onError?: (err: ApiError) => void } = {},
): UsePatchCampaignCreatorResult {
  const qc = useQueryClient();
  const onErrorCallback = options.onError;

  return useMutation<
    CampaignCreator,
    ApiError,
    PatchCampaignCreatorVariables,
    RollbackContext
  >({
    mutationFn: ({ creatorId, patch }) =>
      patchCampaignCreator(campaignId, creatorId, patch),
    onMutate: async ({ creatorId, patch }) => {
      const key = campaignCreatorKeys.list(campaignId);
      await qc.cancelQueries({ queryKey: key });
      const previous = qc.getQueryData<CampaignCreator[]>(key);
      if (previous && patch.ticketSent !== undefined) {
        const next = previous.map((cc) => {
          if (cc.creatorId !== creatorId) return cc;
          return {
            ...cc,
            ticketSentAt: patch.ticketSent
              ? new Date().toISOString()
              : null,
          };
        });
        qc.setQueryData(key, next);
      }
      return { previous };
    },
    onError: (err, _vars, ctx) => {
      const key = campaignCreatorKeys.list(campaignId);
      if (ctx?.previous) {
        qc.setQueryData(key, ctx.previous);
      }
      onErrorCallback?.(err);
    },
    onSettled: () => {
      const key = campaignCreatorKeys.list(campaignId);
      void qc.invalidateQueries({ queryKey: key });
    },
  });
}
