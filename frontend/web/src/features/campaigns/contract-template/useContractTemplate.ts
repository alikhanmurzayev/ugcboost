import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  downloadCampaignContractTemplate,
  uploadCampaignContractTemplate,
  type UploadCampaignContractTemplateResult,
} from "@/api/campaigns";
import { campaignKeys } from "@/shared/constants/queryKeys";

export function useUploadContractTemplate(campaignId: string) {
  const queryClient = useQueryClient();
  return useMutation<UploadCampaignContractTemplateResult, Error, File>({
    mutationFn: (file) => uploadCampaignContractTemplate(campaignId, file),
    onSuccess() {
      void queryClient.invalidateQueries({
        queryKey: campaignKeys.detail(campaignId),
      });
      void queryClient.invalidateQueries({
        queryKey: campaignKeys.contractTemplate(campaignId),
      });
      // List view shows hasContractTemplate per row; refetch so the freshly
      // uploaded template flips the indicator without a manual reload.
      void queryClient.invalidateQueries({ queryKey: campaignKeys.lists() });
    },
  });
}

// triggerDownloadContractTemplate fetches the stored PDF and triggers a
// browser download via an anonymous anchor — keeps the action a one-liner
// from the component without holding the blob URL in component state.
export async function triggerDownloadContractTemplate(
  campaignId: string,
  fileName: string,
): Promise<void> {
  const blob = await downloadCampaignContractTemplate(campaignId);
  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = fileName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    URL.revokeObjectURL(url);
  }
}
