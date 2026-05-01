// Aidana's brand-cabinet layout, copied 1-for-1 from her DashboardLayout in
// aidana/prototype-backup. Two prototype-specific concerns layered on top:
//   - i18n namespaces for her mock locales are prefixed `prototype_*`.
//   - mock-backed counts come from @/_prototype/api/* and prototype queryKeys.
//   - NavLink targets are prefixed with /prototype so navigation stays inside
//     the prototype subtree; logout still redirects to the real /login.
//
// Role view is decoupled from the real auth role: a sticky toggle (persisted
// in localStorage) lets a real admin preview the brand cabinet without an
// extra account, and vice versa. In production the same one-account model
// will use real auth role + per-route guards — this toggle exists only to
// demo both navigation sets from a single login.
import { useEffect, useState } from "react";
import { Link, NavLink, Outlet, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/api/auth";
import { Roles } from "@/shared/constants/roles";
import { getQueueCounts } from "@/_prototype/api/creatorApplications";
import { getCampaignCounts } from "@/_prototype/api/campaigns";
import { ROUTES } from "@/_prototype/routes";
import {
  campaignKeys,
  creatorApplicationKeys,
} from "@/_prototype/queryKeys";

type ViewAs = "admin" | "brand";
const VIEW_AS_KEY = "prototype:viewAs";
const DEMO_BRAND_NAME = "Demo Brand";

interface NavItem {
  to: string;
  label: string;
  badge?: number;
}

interface NavGroup {
  label?: string;
  items: NavItem[];
}

function withPrototypePrefix(path: string): string {
  if (path === "/prototype") return "/prototype";
  return "/prototype/" + path;
}

function readStoredViewAs(): ViewAs | null {
  if (typeof window === "undefined") return null;
  const v = window.localStorage.getItem(VIEW_AS_KEY);
  return v === "admin" || v === "brand" ? v : null;
}

export default function PrototypeLayout() {
  const { t } = useTranslation([
    "auth",
    "brands",
    "audit",
    "dashboard",
    "prototype_creatorApplications",
    "prototype_creators",
    "prototype_campaigns",
  ]);
  const user = useAuthStore((s) => s.user);
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const navigate = useNavigate();

  const [viewAs, setViewAs] = useState<ViewAs>(() => {
    return readStoredViewAs() ?? (user?.role === Roles.ADMIN ? "admin" : "brand");
  });

  useEffect(() => {
    window.localStorage.setItem(VIEW_AS_KEY, viewAs);
  }, [viewAs]);

  const isAdminView = viewAs === "admin";

  const { data: counts } = useQuery({
    queryKey: creatorApplicationKeys.counts(),
    queryFn: getQueueCounts,
    enabled: isAdminView,
  });

  const { data: campaignCounts } = useQuery({
    queryKey: campaignKeys.counts(),
    queryFn: getCampaignCounts,
    enabled: !isAdminView,
  });

  const adminNav: NavGroup[] = [
    {
      items: [{ to: ROUTES.DASHBOARD, label: t("dashboard:title") }],
    },
    {
      label: t("prototype_creatorApplications:navGroup"),
      items: [
        {
          to: ROUTES.CREATOR_APP_VERIFICATION,
          label: t("prototype_creatorApplications:stages.verification.title"),
          badge: counts?.verification,
        },
        {
          to: ROUTES.CREATOR_APP_MODERATION,
          label: t("prototype_creatorApplications:stages.moderation.title"),
          badge: counts?.moderation,
        },
        {
          to: ROUTES.CREATOR_APP_CONTRACTS,
          label: t("prototype_creatorApplications:stages.contracts.title"),
          badge: counts?.contracts,
        },
        {
          to: ROUTES.CREATOR_APP_REJECTED,
          label: t("prototype_creatorApplications:stages.rejected.title"),
        },
      ],
    },
    {
      items: [
        {
          to: ROUTES.CREATORS,
          label: t("prototype_creators:title"),
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
      label: t("prototype_campaigns:navGroup"),
      items: [
        {
          to: ROUTES.CAMPAIGNS_ACTIVE,
          label: t("prototype_campaigns:navStatuses.active"),
          badge: campaignCounts?.active,
        },
        {
          to: ROUTES.CAMPAIGNS_PENDING,
          label: t("prototype_campaigns:navStatuses.pending_moderation"),
          badge: campaignCounts?.pending_moderation,
        },
        {
          to: ROUTES.CAMPAIGNS_REJECTED,
          label: t("prototype_campaigns:navStatuses.rejected"),
          badge: campaignCounts?.rejected,
        },
        {
          to: ROUTES.CAMPAIGNS_DRAFT,
          label: t("prototype_campaigns:navStatuses.draft"),
          badge: campaignCounts?.draft,
        },
        {
          to: ROUTES.CAMPAIGNS_COMPLETED,
          label: t("prototype_campaigns:navStatuses.completed"),
        },
      ],
    },
    {
      items: [{ to: ROUTES.BRANDS, label: t("brands:myBrands") }],
    },
  ];

  const navGroups = isAdminView ? adminNav : brandNav;

  async function handleLogout() {
    try {
      await logout();
    } catch {
      // ignore — clear local state anyway
    }
    clearAuth();
    navigate(ROUTES.LOGIN, { replace: true });
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

        <div className="border-b border-surface-300 px-3 py-3">
          <p className="mb-1 px-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400">
            Просмотр от лица
          </p>
          <div className="flex rounded-button bg-surface-200 p-0.5" role="group">
            <button
              type="button"
              onClick={() => setViewAs("admin")}
              className={`flex-1 rounded-button px-2 py-1 text-xs font-medium transition ${
                isAdminView ? "bg-white text-gray-900 shadow-sm" : "text-gray-600 hover:text-gray-900"
              }`}
              data-testid="prototype-view-admin"
            >
              Админ
            </button>
            <button
              type="button"
              onClick={() => setViewAs("brand")}
              className={`flex-1 rounded-button px-2 py-1 text-xs font-medium transition ${
                !isAdminView ? "bg-white text-gray-900 shadow-sm" : "text-gray-600 hover:text-gray-900"
              }`}
              data-testid="prototype-view-brand"
            >
              Бренд
            </button>
          </div>
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
                {group.items.map((item) => {
                  const href = withPrototypePrefix(item.to);
                  return (
                    <li key={item.to}>
                      <NavLink
                        to={href}
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
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>

        <div className="border-t border-surface-300 px-3 py-4">
          <Link
            to="/"
            className="mb-3 block px-3 text-xs text-gray-500 transition hover:text-gray-700"
            data-testid="prototype-back-to-app"
          >
            ← К основному приложению
          </Link>
          <div className="mb-3 px-3">
            <p className="truncate text-sm font-medium text-gray-900">{user?.email}</p>
            <p className="text-xs text-gray-500">
              {isAdminView ? t("auth:admin") : `${t("auth:brandManager")} · ${DEMO_BRAND_NAME}`}
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
