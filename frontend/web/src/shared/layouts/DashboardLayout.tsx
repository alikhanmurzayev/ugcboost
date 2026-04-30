import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/api/auth";
import { getQueueCounts } from "@/api/creatorApplications";
import { listBrands } from "@/api/brands";
import { getCampaignCounts } from "@/api/campaigns";
import { ROUTES } from "@/shared/constants/routes";
import { Roles } from "@/shared/constants/roles";
import {
  brandKeys,
  campaignKeys,
  creatorApplicationKeys,
} from "@/shared/constants/queryKeys";

interface NavItem {
  to: string;
  label: string;
  badge?: number;
}

interface NavGroup {
  label?: string;
  items: NavItem[];
}

export default function DashboardLayout() {
  const { t } = useTranslation([
    "auth",
    "brands",
    "audit",
    "dashboard",
    "creatorApplications",
    "creators",
    "campaigns",
  ]);
  const user = useAuthStore((s) => s.user);
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const navigate = useNavigate();

  const isAdmin = user?.role === Roles.ADMIN;

  const { data: counts } = useQuery({
    queryKey: creatorApplicationKeys.counts(),
    queryFn: getQueueCounts,
    enabled: isAdmin,
  });

  const { data: myBrandsData } = useQuery({
    queryKey: brandKeys.all(),
    queryFn: () => listBrands(),
    enabled: !!user && !isAdmin,
    staleTime: 5 * 60 * 1000,
  });
  const primaryBrand = myBrandsData?.data.brands[0];

  const { data: campaignCounts } = useQuery({
    queryKey: campaignKeys.counts(),
    queryFn: getCampaignCounts,
    enabled: !!user && !isAdmin,
  });

  const adminNav: NavGroup[] = [
    {
      items: [{ to: ROUTES.DASHBOARD, label: t("dashboard:title") }],
    },
    {
      label: t("creatorApplications:navGroup"),
      items: [
        {
          to: ROUTES.CREATOR_APP_VERIFICATION,
          label: t("creatorApplications:stages.verification.title"),
          badge: counts?.verification,
        },
        {
          to: ROUTES.CREATOR_APP_MODERATION,
          label: t("creatorApplications:stages.moderation.title"),
          badge: counts?.moderation,
        },
        {
          to: ROUTES.CREATOR_APP_CONTRACTS,
          label: t("creatorApplications:stages.contracts.title"),
          badge: counts?.contracts,
        },
        {
          to: ROUTES.CREATOR_APP_REJECTED,
          label: t("creatorApplications:stages.rejected.title"),
        },
      ],
    },
    {
      items: [
        {
          to: ROUTES.CREATORS,
          label: t("creators:title"),
          badge: counts?.creators,
        },
      ],
    },
  ];

  const brandNav: NavGroup[] = [
    {
      items: [{ to: ROUTES.DASHBOARD, label: t("dashboard:title") }],
    },
    {
      label: t("campaigns:navGroup"),
      items: [
        {
          to: ROUTES.CAMPAIGNS_ACTIVE,
          label: t("campaigns:navStatuses.active"),
          badge: campaignCounts?.active,
        },
        {
          to: ROUTES.CAMPAIGNS_PENDING,
          label: t("campaigns:navStatuses.pending_moderation"),
          badge: campaignCounts?.pending_moderation,
        },
        {
          to: ROUTES.CAMPAIGNS_REJECTED,
          label: t("campaigns:navStatuses.rejected"),
          badge: campaignCounts?.rejected,
        },
        {
          to: ROUTES.CAMPAIGNS_DRAFT,
          label: t("campaigns:navStatuses.draft"),
          badge: campaignCounts?.draft,
        },
        {
          to: ROUTES.CAMPAIGNS_COMPLETED,
          label: t("campaigns:navStatuses.completed"),
        },
      ],
    },
    {
      items: [{ to: ROUTES.BRANDS, label: t("brands:myBrands") }],
    },
  ];

  const navGroups = isAdmin ? adminNav : brandNav;

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
      <aside
        className="flex w-60 flex-col border-r border-surface-300 bg-white"
        data-testid="sidebar"
      >
        <div className="flex items-center border-b border-surface-300 px-5 py-3">
          <img src="/logo-ugcboost.png" alt="UGC boost" className="h-12 w-auto" />
        </div>

        <nav className="flex-1 overflow-y-auto px-3 py-4">
          {navGroups.map((group, groupIdx) => (
            <div key={group.label ?? `group-${groupIdx}`} className={groupIdx > 0 ? "mt-6" : ""}>
              {group.label && (
                <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wide text-gray-400">
                  {group.label}
                </p>
              )}
              <ul className="space-y-1">
                {group.items.map((item) => (
                  <li key={item.to}>
                    <NavLink
                      to={item.to}
                      end={item.to === ROUTES.DASHBOARD}
                      className={({ isActive }) =>
                        `flex items-center justify-between rounded-button px-3 py-2 text-sm font-medium transition ${
                          isActive
                            ? "bg-primary-50 text-primary"
                            : "text-gray-600 hover:bg-surface-200 hover:text-gray-900"
                        }`
                      }
                      data-testid={`nav-link-${item.to}`}
                    >
                      <span className="truncate">{item.label}</span>
                      {item.badge !== undefined && item.badge > 0 && (
                        <span
                          className="ml-2 inline-flex min-w-[1.5rem] items-center justify-center rounded-full bg-surface-300 px-2 py-0.5 text-xs font-semibold text-gray-700"
                          data-testid={`nav-badge-${item.to}`}
                        >
                          {item.badge}
                        </span>
                      )}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>

        <div className="border-t border-surface-300 px-3 py-4">
          <div className="mb-3 px-3">
            <p className="truncate text-sm font-medium text-gray-900">{user?.email}</p>
            <p className="text-xs text-gray-500">
              {isAdmin
                ? t("auth:admin")
                : primaryBrand
                  ? `${t("auth:brandManager")} · ${primaryBrand.name}`
                  : t("auth:brandManager")}
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
