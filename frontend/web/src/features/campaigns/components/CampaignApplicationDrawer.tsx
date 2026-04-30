import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import SocialLink from "@/features/creatorApplications/components/SocialLink";
import { CategoryChips } from "@/features/creatorApplications/components/CategoryChip";
import type {
  ApplicationStatus,
  CampaignApplication,
  CreatorReel,
} from "../types";

interface Props {
  applications: CampaignApplication[];
  index: number;
  onClose: () => void;
  onPick: (index: number) => void;
  onAction: (id: string, status: ApplicationStatus) => void;
  onReelClick: (reel: CreatorReel) => void;
  // "applications" — full toolbar (approve / uncertain / reject).
  // "selected" — only "remove from selected" (returns the application to "new").
  mode?: "applications" | "selected";
  // When true the action footer is hidden — used in "selected" mode after the
  // brand has already sent the TZ to creators (removal must go through the
  // replacement flow instead of plain "remove").
  actionsDisabled?: boolean;
}

export default function CampaignApplicationDrawer({
  applications,
  index,
  onClose,
  onPick,
  onAction,
  onReelClick,
  mode = "applications",
  actionsDisabled = false,
}: Props) {
  const { t } = useTranslation("campaigns");
  const app = applications[index];

  useEffect(() => {
    function handle(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key === "ArrowLeft" && index > 0) onPick(index - 1);
      if (e.key === "ArrowRight" && index < applications.length - 1)
        onPick(index + 1);
      if (mode === "applications" && app) {
        if (e.key === "1" || e.key === "x" || e.key === "X")
          onAction(app.id, "rejected");
        if (e.key === "2" || e.key === "?") onAction(app.id, "uncertain");
        if (e.key === "3" || e.key === "v" || e.key === "V")
          onAction(app.id, "approved");
      }
    }
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [applications.length, index, onPick, onClose, app, onAction, mode]);

  if (!app) return null;
  const c = app.creator;
  const canPrev = index > 0;
  const canNext = index < applications.length - 1;

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-gray-900/60 p-4"
      onClick={onClose}
      data-testid="application-drawer"
    >
      <div
        className="relative max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-2xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-surface-200 bg-white px-5 py-3">
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => canPrev && onPick(index - 1)}
              disabled={!canPrev}
              aria-label={t("drawerPrev")}
              className="rounded-button border border-surface-300 px-2 py-1 text-sm text-gray-700 hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-40"
              data-testid="drawer-prev"
            >
              ←
            </button>
            <button
              type="button"
              onClick={() => canNext && onPick(index + 1)}
              disabled={!canNext}
              aria-label={t("drawerNext")}
              className="rounded-button border border-surface-300 px-2 py-1 text-sm text-gray-700 hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-40"
              data-testid="drawer-next"
            >
              →
            </button>
            <span className="text-xs text-gray-500">
              {index + 1} / {applications.length}
            </span>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("drawerClose")}
            className="rounded-full p-1 text-gray-500 hover:bg-surface-100"
            data-testid="drawer-close"
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </header>

        <div className="p-5">
          <div className="flex items-start gap-4">
            <Avatar name={`${c.firstName} ${c.lastName}`} url={c.avatarUrl} size={64} />
            <div className="flex-1">
              <h2 className="text-xl font-semibold text-gray-900">
                {c.lastName} {c.firstName}
              </h2>
              <p className="text-sm text-gray-500">
                {c.age} · {c.city.name}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-3 text-sm">
                {c.socials.map((s) => (
                  <SocialLink
                    key={`${s.platform}-${s.handle}`}
                    platform={s.platform}
                    handle={s.handle}
                    showHandle
                  />
                ))}
              </div>
              <div className="mt-2">
                <CategoryChips categories={c.categories} />
              </div>
            </div>
            <StatusBadge status={app.status} />
          </div>

          <div className="mt-5 grid grid-cols-3 gap-3">
            <Metric label={t("metricsFollowers")} value={compact(c.metrics.followers)} />
            <Metric label={t("metricsAvgViews")} value={compact(c.metrics.avgViews)} />
            <Metric label={t("metricsEr")} value={`${c.metrics.er}%`} />
          </div>

          <h3 className="mt-6 text-xs font-semibold uppercase tracking-wide text-gray-500">
            {t("recentReels")}
          </h3>
          <div className="mt-2 grid grid-cols-3 gap-2 sm:grid-cols-6">
            {c.recentReels.map((r) => (
              <video
                key={r.id}
                src={r.videoUrl}
                poster={r.thumbnailUrl}
                muted
                autoPlay
                playsInline
                loop
                preload="metadata"
                onClick={() => onReelClick(r)}
                className="aspect-[9/16] w-full cursor-pointer rounded-card border border-surface-200 bg-surface-100 object-cover transition hover:opacity-90"
              />
            ))}
          </div>
        </div>

        <footer className="sticky bottom-0 z-10 flex items-center justify-end gap-2 border-t border-surface-200 bg-white px-5 py-3">
          {mode === "selected" ? (
            !actionsDisabled && (
              <ActionPill
                variant="reject"
                active={false}
                onClick={() => onAction(app.id, "new")}
                label={t("selectedActionRemove")}
                testid="drawer-action-remove"
              />
            )
          ) : (
            <>
              <ActionPill
                variant="reject"
                active={app.status === "rejected"}
                onClick={() =>
                  onAction(
                    app.id,
                    app.status === "rejected" ? "new" : "rejected",
                  )
                }
                label={t("applicationsActionReject")}
                testid="drawer-action-reject"
              />
              <ActionPill
                variant="uncertain"
                active={app.status === "uncertain"}
                onClick={() =>
                  onAction(
                    app.id,
                    app.status === "uncertain" ? "new" : "uncertain",
                  )
                }
                label={t("applicationsActionUncertain")}
                testid="drawer-action-uncertain"
              />
              <ActionPill
                variant="approve"
                active={app.status === "approved"}
                onClick={() =>
                  onAction(
                    app.id,
                    app.status === "approved" ? "new" : "approved",
                  )
                }
                label={t("applicationsActionApprove")}
                testid="drawer-action-approve"
              />
            </>
          )}
        </footer>
      </div>
    </div>
  );
}

function Avatar({
  name,
  url,
  size = 48,
}: {
  name: string;
  url?: string;
  size?: number;
}) {
  const style = { width: size, height: size };
  if (url) {
    return (
      <img
        src={url}
        alt={name}
        style={style}
        className="shrink-0 rounded-full object-cover"
      />
    );
  }
  const initials = name
    .split(" ")
    .map((s) => s[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
  return (
    <div
      style={style}
      className="flex shrink-0 items-center justify-center rounded-full bg-primary-50 text-base font-semibold text-primary"
    >
      {initials}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-card border border-surface-200 bg-surface-50 px-3 py-2 text-center">
      <p className="text-lg font-semibold text-gray-900 tabular-nums">
        {value}
      </p>
      <p className="text-xs text-gray-500">{label}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: ApplicationStatus }) {
  const { t } = useTranslation("campaigns");
  if (status === "new") return null;
  const map = {
    approved: ["bg-emerald-100 text-emerald-700", "applicationsApproved"],
    rejected: ["bg-surface-200 text-gray-600", "applicationsRejected"],
    uncertain: ["bg-amber-100 text-amber-700", "applicationsUncertain"],
  } as const;
  const [cls, key] = map[status];
  return (
    <span
      className={`shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`}
    >
      {t(key)}
    </span>
  );
}

function ActionPill({
  variant,
  active,
  onClick,
  label,
  testid,
}: {
  variant: "approve" | "reject" | "uncertain";
  active: boolean;
  onClick: () => void;
  label: string;
  testid: string;
}) {
  const palette = {
    approve: active
      ? "bg-emerald-500 text-white border-emerald-500"
      : "bg-emerald-50 text-emerald-700 border-emerald-200 hover:bg-emerald-100",
    reject: active
      ? "bg-red-500 text-white border-red-500"
      : "bg-red-50 text-red-700 border-red-200 hover:bg-red-100",
    uncertain: active
      ? "bg-amber-500 text-white border-amber-500"
      : "bg-amber-50 text-amber-700 border-amber-200 hover:bg-amber-100",
  }[variant];
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      data-testid={testid}
      className={`rounded-full border px-4 py-2 text-sm font-semibold transition ${palette}`}
    >
      {label}
    </button>
  );
}

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${Math.round(n / 100) / 10}K`;
  return String(n);
}
