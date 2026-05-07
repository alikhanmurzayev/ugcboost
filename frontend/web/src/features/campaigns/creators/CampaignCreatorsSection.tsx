import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import type { Campaign } from "@/api/campaigns";
import { SEARCH_PARAMS } from "@/shared/constants/routes";
import {
  useCampaignCreators,
  type CampaignCreatorRow,
} from "./hooks/useCampaignCreators";
import CampaignCreatorsTable from "./CampaignCreatorsTable";

interface CampaignCreatorsSectionProps {
  campaign: Campaign;
}

export default function CampaignCreatorsSection({
  campaign,
}: CampaignCreatorsSectionProps) {
  const { t } = useTranslation("campaigns");
  const [searchParams, setSearchParams] = useSearchParams();
  const { rows, total, isLoading, isError, refetch } = useCampaignCreators(
    campaign.id,
    { enabled: !campaign.isDeleted },
  );

  if (campaign.isDeleted) return null;

  const selectedCreatorId = searchParams.get(SEARCH_PARAMS.CREATOR_ID);

  function handleRowClick(row: CampaignCreatorRow) {
    // Soft-deleted creators have no profile and getCreator would 404 in the
    // drawer. Skip the click; the placeholder + tooltip already communicate
    // that the row is inert.
    if (!row.creator) return;
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.set(SEARCH_PARAMS.CREATOR_ID, row.campaignCreator.creatorId);
      return np;
    });
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
          disabled
          title={t("campaignCreators.addDisabledTooltip")}
          className="rounded-button border border-surface-300 px-3 py-1.5 text-sm text-gray-700 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="campaign-creators-add-button"
        >
          {t("campaignCreators.addButton")}
        </button>
      </div>

      {isLoading ? (
        <Spinner className="mt-6" />
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
          emptyMessage={t("campaignCreators.empty")}
        />
      )}
    </section>
  );
}
