import { useTranslation } from "react-i18next";

export default function CampaignsStubPage() {
  const { t } = useTranslation("common");
  return (
    <div data-testid="campaigns-stub">
      <h1 className="text-2xl font-bold text-gray-900">{t("navCampaigns")}</h1>
      <p className="mt-2 text-sm text-gray-500">{t("comingSoon")}</p>
    </div>
  );
}
