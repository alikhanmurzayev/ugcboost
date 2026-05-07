import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import {
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import { ApiError } from "@/api/client";
import type { Campaign } from "@/api/campaigns";
import { removeCampaignCreator } from "@/api/campaignCreators";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";
import { SEARCH_PARAMS } from "@/shared/constants/routes";
import {
  useCampaignCreators,
  type CampaignCreatorRow,
} from "./hooks/useCampaignCreators";
import CampaignCreatorsTable from "./CampaignCreatorsTable";
import AddCreatorsDrawer from "./AddCreatorsDrawer";
import RemoveCreatorConfirm from "./RemoveCreatorConfirm";

interface CampaignCreatorsSectionProps {
  campaign: Campaign;
}

export default function CampaignCreatorsSection({
  campaign,
}: CampaignCreatorsSectionProps) {
  const { t } = useTranslation("campaigns");
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const { rows, total, existingCreatorIds, isLoading, isError, refetch } =
    useCampaignCreators(campaign.id, { enabled: !campaign.isDeleted });

  const [isAddOpen, setIsAddOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<CampaignCreatorRow | null>(
    null,
  );
  const [removeError, setRemoveError] = useState<string | null>(null);
  // Double-submit guard: external flag mirrors `isPending` but is also held
  // during the synchronous gap between rapid clicks before React re-renders
  // the disabled-button state. Sibling pattern to AddCreatorsDrawer.
  const [isRemoveSubmitting, setIsRemoveSubmitting] = useState(false);

  const removeMutation = useMutation({
    mutationFn: ({
      campaignId,
      creatorId,
    }: {
      campaignId: string;
      creatorId: string;
    }) => removeCampaignCreator(campaignId, creatorId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: campaignCreatorKeys.all(),
      });
      setRemoveTarget(null);
      setRemoveError(null);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.status === 404) {
        void queryClient.invalidateQueries({
          queryKey: campaignCreatorKeys.all(),
        });
        setRemoveTarget(null);
        setRemoveError(null);
        return;
      }
      if (
        apiErr?.status === 422 &&
        apiErr.code === "CAMPAIGN_CREATOR_AGREED"
      ) {
        void queryClient.invalidateQueries({
          queryKey: campaignCreatorKeys.all(),
        });
        setRemoveError(t("campaignCreators.errors.creatorAgreed"));
        return;
      }
      setRemoveError(t("campaignCreators.errors.removeFailed"));
    },
    onSettled: () => {
      setIsRemoveSubmitting(false);
    },
  });

  if (campaign.isDeleted) return null;

  const selectedCreatorId = searchParams.get(SEARCH_PARAMS.CREATOR_ID);

  function handleRowClick(row: CampaignCreatorRow) {
    if (!row.creator) return;
    if (selectedCreatorId === row.campaignCreator.creatorId) return;
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.set(SEARCH_PARAMS.CREATOR_ID, row.campaignCreator.creatorId);
      return np;
    });
  }

  function handleRemoveRequest(row: CampaignCreatorRow) {
    // Ignore trash clicks while a previous remove is still in-flight; the
    // dialog would otherwise re-open with the new creator's name while the
    // earlier mutation's onSettled is still pending.
    if (isRemoveSubmitting || removeMutation.isPending) return;
    setRemoveError(null);
    setRemoveTarget(row);
  }

  function handleRemoveConfirm() {
    if (!removeTarget) return;
    if (isRemoveSubmitting || removeMutation.isPending) return;
    setIsRemoveSubmitting(true);
    removeMutation.mutate({
      campaignId: campaign.id,
      creatorId: removeTarget.campaignCreator.creatorId,
    });
  }

  function handleRemoveClose() {
    if (isRemoveSubmitting || removeMutation.isPending) return;
    setRemoveTarget(null);
    setRemoveError(null);
  }

  function handleAddClose() {
    setIsAddOpen(false);
  }

  return (
    <section
      className="mt-6 rounded-card border border-surface-300 bg-white p-6"
      data-testid="campaign-creators-section"
    >
      <div className="flex items-center justify-between">
        <h2 className="flex items-baseline gap-3 text-lg font-bold text-gray-900">
          {t("campaignCreators.title")}
          {!isLoading && !isError && total > 0 && (
            <span
              className="text-sm font-medium text-gray-400"
              data-testid="campaign-creators-counter"
            >
              {t("campaignCreators.count", { count: total })}
            </span>
          )}
        </h2>
        <button
          type="button"
          onClick={() => setIsAddOpen(true)}
          className="rounded-button border border-surface-300 px-3 py-1.5 text-sm text-gray-700 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="campaign-creators-add-button"
        >
          {t("campaignCreators.addButton")}
        </button>
      </div>

      {isLoading ? (
        <div data-testid="campaign-creators-loading">
          <Spinner className="mt-6" />
        </div>
      ) : isError ? (
        <ErrorState
          message={t("campaignCreators.loadError")}
          onRetry={refetch}
        />
      ) : (
        <CampaignCreatorsTable
          rows={rows}
          selectedKey={selectedCreatorId ?? undefined}
          onRowClick={handleRowClick}
          onRemove={handleRemoveRequest}
          emptyMessage={t("campaignCreators.empty")}
        />
      )}

      <AddCreatorsDrawer
        open={isAddOpen}
        campaignId={campaign.id}
        existingCreatorIds={existingCreatorIds}
        onClose={handleAddClose}
      />

      <RemoveCreatorConfirm
        open={!!removeTarget}
        creatorName={removeTargetName(removeTarget, t)}
        isLoading={isRemoveSubmitting || removeMutation.isPending}
        error={removeError ?? undefined}
        onClose={handleRemoveClose}
        onConfirm={handleRemoveConfirm}
      />
    </section>
  );
}

function removeTargetName(
  target: CampaignCreatorRow | null,
  t: (key: string) => string,
): string {
  if (!target) return "";
  if (!target.creator) return t("campaignCreators.creatorDeleted");
  return `${target.creator.lastName} ${target.creator.firstName}`;
}
