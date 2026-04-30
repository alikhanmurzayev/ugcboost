import { Link, NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { getQueueCounts } from "@/_prototype/api/creatorApplications";
import { getCampaignCounts } from "@/_prototype/api/campaigns";
import { ROUTES } from "@/_prototype/routes";
import {
  campaignKeys,
  creatorApplicationKeys,
} from "@/_prototype/queryKeys";

interface NavItem {
  to: string;
  label: string;
  badge?: number;
}

interface NavGroup {
  label?: string;
  items: NavItem[];
}

export default function PrototypeLayout() {
  const { t } = useTranslation([
    "prototype_creatorApplications",
    "prototype_creators",
    "prototype_campaigns",
  ]);

  const { data: appCounts } = useQuery({
    queryKey: creatorApplicationKeys.counts(),
    queryFn: getQueueCounts,
  });

  const { data: campaignCounts } = useQuery({
    queryKey: campaignKeys.counts(),
    queryFn: getCampaignCounts,
  });

  const navGroups: NavGroup[] = [
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
      label: t("prototype_creatorApplications:navGroup"),
      items: [
        {
          to: ROUTES.CREATOR_APP_VERIFICATION,
          label: t("prototype_creatorApplications:stages.verification.title"),
          badge: appCounts?.verification,
        },
        {
          to: ROUTES.CREATOR_APP_MODERATION,
          label: t("prototype_creatorApplications:stages.moderation.title"),
          badge: appCounts?.moderation,
        },
        {
          to: ROUTES.CREATOR_APP_CONTRACTS,
          label: t("prototype_creatorApplications:stages.contracts.title"),
          badge: appCounts?.contracts,
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
          badge: appCounts?.creators,
        },
      ],
    },
  ];

  return (
    <div className="flex min-h-screen bg-surface-100">
      <aside
        className="flex w-60 flex-col border-r border-surface-300 bg-white"
        data-testid="prototype-sidebar"
      >
        <div className="flex flex-col gap-1 border-b border-surface-300 px-5 py-3">
          <img src="/logo-ugcboost.png" alt="UGC boost" className="h-12 w-auto" />
          <span className="text-[10px] font-semibold uppercase tracking-wider text-amber-700">
            🧪 Прототип
          </span>
        </div>

        <nav className="flex-1 overflow-y-auto px-3 py-4">
          {navGroups.map((group, groupIdx) => (
            <div
              key={group.label ?? `group-${groupIdx}`}
              className={groupIdx > 0 ? "mt-6" : ""}
            >
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
                      className={({ isActive }) =>
                        `flex items-center justify-between rounded-button px-3 py-2 text-sm font-medium transition ${
                          isActive
                            ? "bg-primary-50 text-primary"
                            : "text-gray-600 hover:bg-surface-200 hover:text-gray-900"
                        }`
                      }
                      data-testid={`prototype-nav-link-${item.to}`}
                    >
                      <span className="truncate">{item.label}</span>
                      {item.badge !== undefined && item.badge > 0 && (
                        <span
                          className="ml-2 inline-flex min-w-[1.5rem] items-center justify-center rounded-full bg-surface-300 px-2 py-0.5 text-xs font-semibold text-gray-700"
                          data-testid={`prototype-nav-badge-${item.to}`}
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
          <Link
            to="/"
            className="block rounded-button px-3 py-2 text-left text-sm text-gray-600 transition hover:bg-surface-200 hover:text-gray-900"
            data-testid="prototype-back-to-app"
          >
            ← К основному приложению
          </Link>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto p-8">
        <div className="mb-4 rounded-card border border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-900">
          🧪 Это прототип бренд-кабинета. Все данные мокнутые. По мере
          реализации фич будут переноситься в основное приложение.
        </div>
        <Outlet />
      </main>
    </div>
  );
}
