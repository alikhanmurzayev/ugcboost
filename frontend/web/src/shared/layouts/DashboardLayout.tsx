import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/api/auth";
import { ROUTES } from "@/shared/constants/routes";
import { Roles } from "@/shared/constants/roles";

export default function DashboardLayout() {
  const { t } = useTranslation(["auth", "brands", "audit", "dashboard"]);
  const user = useAuthStore((s) => s.user);
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const navigate = useNavigate();

  const adminNav = [
    { to: ROUTES.DASHBOARD, label: t("dashboard:title") },
    { to: ROUTES.BRANDS, label: t("brands:title") },
    { to: ROUTES.AUDIT, label: t("audit:title") },
  ];

  const brandNav = [
    { to: ROUTES.DASHBOARD, label: t("dashboard:title") },
    { to: ROUTES.BRANDS, label: t("brands:myBrands") },
  ];

  const nav = user?.role === Roles.ADMIN ? adminNav : brandNav;

  async function handleLogout() {
    try {
      await logout();
    } catch {
      // ignore — clear local state anyway
    }
    clearAuth();
    navigate("/" + ROUTES.LOGIN, { replace: true });
  }

  return (
    <div className="flex min-h-screen bg-surface-100">
      <aside className="flex w-60 flex-col border-r border-surface-300 bg-white" data-testid="sidebar">
        <div className="border-b border-surface-300 px-5 py-4">
          <span className="text-lg font-bold text-gray-900">UGCBoost</span>
        </div>

        <nav className="flex-1 px-3 py-4">
          <ul className="space-y-1">
            {nav.map((item) => (
              <li key={item.to}>
                <NavLink
                  to={item.to}
                  end={item.to === ROUTES.DASHBOARD}
                  className={({ isActive }) =>
                    `block rounded-button px-3 py-2 text-sm font-medium transition ${
                      isActive
                        ? "bg-primary-50 text-primary"
                        : "text-gray-600 hover:bg-surface-200 hover:text-gray-900"
                    }`
                  }
                  data-testid={`nav-link-${item.to}`}
                >
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        <div className="border-t border-surface-300 px-3 py-4">
          <div className="mb-3 px-3">
            <p className="truncate text-sm font-medium text-gray-900">{user?.email}</p>
            <p className="text-xs text-gray-500">
              {user?.role === Roles.ADMIN ? t("auth:admin") : t("auth:brandManager")}
            </p>
          </div>
          <button
            onClick={handleLogout}
            className="w-full rounded-button px-3 py-2 text-left text-sm text-gray-600 transition hover:bg-surface-200 hover:text-gray-900"
            data-testid="logout-button"
          >
            {t("auth:logout")}
          </button>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto p-8">
        <Outlet />
      </main>
    </div>
  );
}
