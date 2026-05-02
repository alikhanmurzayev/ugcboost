import { useTranslation } from "react-i18next";

export default function ContractsPage() {
  const { t } = useTranslation(["creatorApplications", "common"]);
  return (
    <div data-testid="creator-applications-contracts-stub">
      <h1 className="text-2xl font-bold text-gray-900">
        {t("creatorApplications:stages.contracts.title")}
      </h1>
      <p className="mt-2 text-sm text-gray-500">{t("common:comingSoon")}</p>
    </div>
  );
}
