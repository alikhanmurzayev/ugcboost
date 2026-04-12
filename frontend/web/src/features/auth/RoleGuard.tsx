import { Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/auth";
import type { UserRole } from "@/shared/constants/roles";

interface RoleGuardProps {
  allowedRoles: UserRole[];
}

export default function RoleGuard({ allowedRoles }: RoleGuardProps) {
  const { t } = useTranslation("common");
  const user = useAuthStore((s) => s.user);

  if (!user || !allowedRoles.includes(user.role)) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <p className="text-lg font-medium text-gray-900">{t("noAccess")}</p>
        <p className="mt-1 text-sm text-gray-500">{t("noAccessDescription")}</p>
      </div>
    );
  }

  return <Outlet />;
}
