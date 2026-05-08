import { useMemo, useState } from "react";
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
import {
  removeCampaignCreator,
  type CampaignCreatorStatus,
} from "@/api/campaignCreators";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";
import {
  CAMPAIGN_CREATOR_GROUP_ORDER,
  CAMPAIGN_CREATOR_STATUS,
} from "@/shared/constants/campaignCreatorStatus";
import { SEARCH_PARAMS } from "@/shared/constants/routes";
import {
  useCampaignCreators,
  type CampaignCreatorRow,
} from "./hooks/useCampaignCreators";
import { useCampaignNotifyMutations } from "./hooks/useCampaignNotifyMutations";
import CampaignCreatorGroupSection from "./CampaignCreatorGroupSection";
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

  const notifyMutations = useCampaignNotifyMutations(campaign.id);

  const [isAddOpen, setIsAddOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<CampaignCreatorRow | null>(
    null,
  );
  const [removeError, setRemoveError] = useState<string | null>(null);
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
        queryKey: campaignCreatorKeys.list(campaign.id),
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

  const groupedRows = useMemo(() => {
    const acc: Record<CampaignCreatorStatus, CampaignCreatorRow[]> = {
      planned: [],
      invited: [],
      declined: [],
      agreed: [],
    };
    for (const row of rows) {
      acc[row.campaignCreator.status].push(row);
    }
    return acc;
  }, [rows]);

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
      ) : total === 0 ? (
        <p
          className="mt-6 text-gray-500"
          data-testid="campaign-creators-empty-all"
        >
          {t("campaignCreators.emptyAll")}
        </p>
      ) : (
        CAMPAIGN_CREATOR_GROUP_ORDER.map((status) => {
          const groupRows = groupedRows[status];
          if (groupRows.length === 0) return null;
          const action = actionForStatus(status, notifyMutations, t);
          return (
            <CampaignCreatorGroupSection
              key={status}
              status={status}
              campaignId={campaign.id}
              title={t(`campaignCreators.groups.${status}`)}
              rows={groupRows}
              actionLabel={action.actionLabel}
              mutation={action.mutation}
              onRemove={handleRemoveRequest}
              drawerSelectedCreatorId={selectedCreatorId ?? undefined}
              onRowClick={handleRowClick}
            />
          );
        })
      )}

      {isAddOpen && (
        <AddCreatorsDrawer
          open={isAddOpen}
          campaignId={campaign.id}
          existingCreatorIds={existingCreatorIds}
          onClose={handleAddClose}
        />
      )}

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

type NotifyMutations = ReturnType<typeof useCampaignNotifyMutations>;

function actionForStatus(
  status: CampaignCreatorStatus,
  mutations: NotifyMutations,
  t: (key: string) => string,
): {
  actionLabel?: string;
  mutation?: NotifyMutations["notify"] | NotifyMutations["remind"];
} {
  if (
    status === CAMPAIGN_CREATOR_STATUS.PLANNED ||
    status === CAMPAIGN_CREATOR_STATUS.DECLINED
  ) {
    return {
      actionLabel: t("campaignCreators.notifyButton"),
      mutation: mutations.notify,
    };
  }
  if (status === CAMPAIGN_CREATOR_STATUS.INVITED) {
    return {
      actionLabel: t("campaignCreators.remindButton"),
      mutation: mutations.remind,
    };
  }
  return {};
}

function removeTargetName(
  target: CampaignCreatorRow | null,
  t: (key: string) => string,
): string {
  if (!target) return "";
  if (!target.creator) return t("campaignCreators.creatorDeleted");
  return `${target.creator.lastName} ${target.creator.firstName}`;
}
