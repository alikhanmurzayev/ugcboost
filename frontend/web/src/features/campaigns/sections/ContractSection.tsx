import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getCampaign, listCampaignApplications } from "@/api/campaigns";
import { campaignKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import CostSummary from "../components/CostSummary";
import ContractWizard from "../components/ContractWizard";

// Until the brand profile is implemented, the brand city is hard-coded.
// Used to compute logistics: 1500 ₸ same city / 2500 ₸ another city.
const BRAND_CITY = "almaty";

interface Props {
  campaignId: string;
}

export default function ContractSection({ campaignId }: Props) {
  const { t } = useTranslation("campaigns");
  const [contractOpen, setContractOpen] = useState(false);
  const [contractDone, setContractDone] = useState(false);

  const campaignQuery = useQuery({
    queryKey: campaignKeys.detail(campaignId),
    queryFn: () => getCampaign(campaignId),
  });
  const applicationsQuery = useQuery({
    queryKey: campaignKeys.applications(campaignId),
    queryFn: () => listCampaignApplications(campaignId),
  });

  const accepted = useMemo(() => {
    return (applicationsQuery.data ?? []).filter(
      (a) => a.status === "approved" && a.tzStatus === "accepted",
    );
  }, [applicationsQuery.data]);

  const visible = useMemo(() => {
    return (applicationsQuery.data ?? []).filter(
      (a) => a.status === "approved" && a.tzStatus !== "replaced",
    );
  }, [applicationsQuery.data]);

  if (campaignQuery.isLoading || applicationsQuery.isLoading)
    return <Spinner className="mt-12" />;
  if (campaignQuery.isError || !campaignQuery.data)
    return (
      <ErrorState
        message={t("loadError")}
        onRetry={() => void campaignQuery.refetch()}
      />
    );

  const campaign = campaignQuery.data;
  const target = campaign.creatorsCount;
  const allAccepted =
    visible.length > 0 && visible.every((a) => a.tzStatus === "accepted");
  const ready = allAccepted && accepted.length === target;

  if (!ready) {
    return (
      <div
        className="rounded-2xl border border-dashed border-surface-300 bg-white p-8 text-center"
        data-testid="contract-locked"
      >
        <p className="text-base font-semibold text-gray-900">
          {t("contract.lockedTitle")}
        </p>
        <p className="mt-2 text-sm text-gray-500">
          {t("contract.lockedHint", {
            accepted: accepted.length,
            target,
          })}
        </p>
      </div>
    );
  }

  return (
    <div data-testid="contract-section">
      <CostSummary
        campaign={campaign}
        approved={accepted}
        brandCity={BRAND_CITY}
      />

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-surface-200 bg-white p-5 shadow-sm">
        <div>
          <p className="text-sm font-semibold text-gray-900">
            {contractDone
              ? t("contractActions.signed")
              : t("contractActions.cta")}
          </p>
          <p className="mt-0.5 text-xs text-gray-500">
            {contractDone
              ? t("contractActions.signedHint")
              : t("contractActions.ctaHint")}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setContractOpen(true)}
          disabled={contractDone}
          className="rounded-button bg-primary px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="open-contract-wizard"
        >
          {contractDone
            ? t("contractActions.signedButton")
            : t("contractActions.button")}
        </button>
      </div>

      {contractOpen && (
        <ContractWizard
          campaign={campaign}
          approved={accepted}
          brandCity={BRAND_CITY}
          brandName={campaign.brandName}
          onClose={() => setContractOpen(false)}
          onSubmit={() => {
            setContractOpen(false);
            setContractDone(true);
          }}
        />
      )}
    </div>
  );
}
