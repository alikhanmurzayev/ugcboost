import { Outlet } from "react-router-dom";
import { useAuthStore } from "@/stores/auth";
import type { UserRole } from "@/shared/constants/roles";

interface RoleGuardProps {
  allowedRoles: UserRole[];
}

export default function RoleGuard({ allowedRoles }: RoleGuardProps) {
  const user = useAuthStore((s) => s.user);

  if (!user || !allowedRoles.includes(user.role as UserRole)) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <p className="text-lg font-medium text-gray-900">Нет доступа</p>
        <p className="mt-1 text-sm text-gray-500">
          У вас нет прав для просмотра этой страницы
        </p>
      </div>
    );
  }

  return <Outlet />;
}
