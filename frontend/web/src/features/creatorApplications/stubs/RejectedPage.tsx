import { useTranslation } from "react-i18next";

export default function RejectedPage() {
  const { t } = useTranslation(["creatorApplications", "common"]);
  return (
    <div data-testid="creator-applications-rejected-stub">
      <h1 className="text-2xl font-bold text-gray-900">
        {t("creatorApplications:stages.rejected.title")}
      </h1>
      <p className="mt-2 text-sm text-gray-500">{t("common:comingSoon")}</p>
    </div>
  );
}
