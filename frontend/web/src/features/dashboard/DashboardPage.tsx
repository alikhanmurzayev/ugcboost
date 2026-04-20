import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/auth";

export default function DashboardPage() {
  const { t } = useTranslation("dashboard");
  const user = useAuthStore((s) => s.user);

  return (
    <div data-testid="dashboard-page">
      <h1 className="text-2xl font-bold text-gray-900">{t("title")}</h1>
      <p className="mt-2 text-gray-500">
        {t("welcome")}, {user?.email}
      </p>
    </div>
  );
}
