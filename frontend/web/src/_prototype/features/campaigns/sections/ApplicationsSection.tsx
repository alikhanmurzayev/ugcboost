import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  listCampaignApplications,
  setApplicationStatus,
} from "@/_prototype/api/campaigns";
import { campaignKeys } from "@/_prototype/queryKeys";
import Spinner from "@/_prototype/shared/components/Spinner";
import ErrorState from "@/_prototype/shared/components/ErrorState";
import SocialLink from "@/_prototype/features/creatorApplications/components/SocialLink";
import { CategoryChips } from "@/_prototype/features/creatorApplications/components/CategoryChip";
import CampaignApplicationDrawer from "../components/CampaignApplicationDrawer";
import ReelLightbox from "../components/ReelLightbox";
import type {
  ApplicationStatus,
  CampaignApplication,
  CreatorReel,
} from "../types";

interface Props {
  campaignId: string;
}

export default function ApplicationsSection({ campaignId }: Props) {
  const { t } = useTranslation("prototype_campaigns");
  const queryClient = useQueryClient();
  const [openIndex, setOpenIndex] = useState<number | null>(null);
  const [lightboxReel, setLightboxReel] = useState<CreatorReel | null>(null);

  const query = useQuery({
    queryKey: campaignKeys.applications(campaignId),
    queryFn: () => listCampaignApplications(campaignId),
  });

  const mutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: ApplicationStatus }) =>
      setApplicationStatus(id, status),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: campaignKeys.applications(campaignId),
      });
    },
  });

  const list = useMemo(() => query.data ?? [], [query.data]);
  const counts = useMemo(
    () => ({
      total: list.length,
      approved: list.filter((a) => a.status === "approved").length,
      rejected: list.filter((a) => a.status === "rejected").length,
      uncertain: list.filter((a) => a.status === "uncertain").length,
    }),
    [list],
  );

  if (query.isLoading) return <Spinner className="mt-12" />;
  if (query.isError)
    return (
      <ErrorState
        message={t("loadError")}
        onRetry={() => void query.refetch()}
      />
    );

  if (list.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-surface-300 bg-white p-8 text-center text-sm text-gray-500">
        {t("applicationsEmpty")}
      </div>
    );
  }

  function setStatus(id: string, status: ApplicationStatus) {
    mutation.mutate({ id, status });
  }

  return (
    <div data-testid="applications-section">
      <div className="mb-4 flex flex-wrap items-baseline gap-3 text-sm text-gray-600">
        <span>
          {t("applicationsTotal")}: <strong>{counts.total}</strong>
        </span>
        <span className="text-emerald-700">
          ✓ <strong>{counts.approved}</strong>
        </span>
        <span className="text-amber-700">
          ❓ <strong>{counts.uncertain}</strong>
        </span>
        <span className="text-gray-500">
          ✕ <strong>{counts.rejected}</strong>
        </span>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        {list.map((app, i) => (
          <ApplicationCard
            key={app.id}
            application={app}
            disabled={mutation.isPending}
            onClick={() => setOpenIndex(i)}
            onAction={(status) => setStatus(app.id, status)}
            onReelClick={setLightboxReel}
          />
        ))}
      </div>

      {openIndex !== null && (
        <CampaignApplicationDrawer
          applications={list}
          index={openIndex}
          onClose={() => setOpenIndex(null)}
          onPick={(i) => setOpenIndex(i)}
          onAction={setStatus}
          onReelClick={setLightboxReel}
        />
      )}

      {lightboxReel && (
        <ReelLightbox
          reel={lightboxReel}
          onClose={() => setLightboxReel(null)}
        />
      )}
    </div>
  );
}

function ApplicationCard({
  application,
  onClick,
  onAction,
  onReelClick,
  disabled,
}: {
  application: CampaignApplication;
  onClick: () => void;
  onAction: (status: ApplicationStatus) => void;
  onReelClick: (reel: CreatorReel) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation("prototype_campaigns");
  const c = application.creator;
  const isRejected = application.status === "rejected";
  const isApproved = application.status === "approved";
  const isUncertain = application.status === "uncertain";
  const isDecided = isRejected || isApproved || isUncertain;

  function stop(handler: () => void) {
    return (e: React.MouseEvent | React.KeyboardEvent) => {
      e.stopPropagation();
      handler();
    };
  }

  return (
    <article
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
      className={`group cursor-pointer rounded-2xl border bg-white p-4 shadow-sm transition ${
        isDecided ? "hover:shadow-md" : "hover:border-primary/40 hover:shadow-md"
      } ${
        isRejected
          ? "border-surface-200 opacity-50"
          : isApproved
            ? "border-emerald-200 ring-1 ring-emerald-200"
            : isUncertain
              ? "border-amber-200 ring-1 ring-amber-200"
              : "border-surface-200"
      }`}
      data-testid={`application-card-${application.id}`}
    >
      <div className="flex items-start gap-3">
        <Avatar name={`${c.firstName} ${c.lastName}`} url={c.avatarUrl} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate font-semibold text-gray-900">
                {c.lastName} {c.firstName}
              </p>
              <p className="text-xs text-gray-500">
                {c.age} · {c.city.name}
              </p>
            </div>
            <div
              className={`flex shrink-0 items-center gap-1 ${
                isRejected
                  ? "[&_button]:grayscale [&_button:hover]:grayscale-0"
                  : ""
              }`}
              onClick={(e) => e.stopPropagation()}
            >
              <ActionButton
                variant="reject"
                active={isRejected}
                disabled={disabled}
                onClick={stop(() =>
                  onAction(isRejected ? "new" : "rejected"),
                )}
                ariaLabel={t("applicationsActionReject")}
                testid={`application-action-reject-${application.id}`}
              />
              <ActionButton
                variant="uncertain"
                active={isUncertain}
                disabled={disabled}
                onClick={stop(() =>
                  onAction(isUncertain ? "new" : "uncertain"),
                )}
                ariaLabel={t("applicationsActionUncertain")}
                testid={`application-action-uncertain-${application.id}`}
              />
              <ActionButton
                variant="approve"
                active={isApproved}
                disabled={disabled}
                onClick={stop(() =>
                  onAction(isApproved ? "new" : "approved"),
                )}
                ariaLabel={t("applicationsActionApprove")}
                testid={`application-action-approve-${application.id}`}
              />
            </div>
          </div>

          <div
            className="mt-1 flex flex-wrap items-center gap-3 text-sm text-gray-700"
            onClick={(e) => e.stopPropagation()}
          >
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
      </div>

      <div className="mt-3 grid grid-cols-3 gap-2 rounded-card border border-surface-200 bg-surface-50 px-3 py-2 text-center text-xs">
        <Metric value={compact(c.metrics.followers)} label={t("metricsFollowers")} />
        <Metric value={compact(c.metrics.avgViews)} label={t("metricsAvgViews")} />
        <Metric value={`${c.metrics.er}%`} label={t("metricsEr")} />
      </div>

      <div
        className="mt-3 grid grid-cols-6 gap-1"
        onClick={(e) => e.stopPropagation()}
      >
        {c.recentReels.map((r) => (
          <ReelTile key={r.id} reel={r} onClick={() => onReelClick(r)} />
        ))}
      </div>
    </article>
  );
}

function ReelTile({
  reel,
  onClick,
}: {
  reel: CreatorReel;
  onClick: () => void;
}) {
  return (
    <video
      src={reel.videoUrl}
      poster={reel.thumbnailUrl}
      muted
      autoPlay
      playsInline
      loop
      preload="metadata"
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className="aspect-[9/16] w-full cursor-pointer rounded bg-surface-100 object-cover transition hover:opacity-90"
    />
  );
}

function Metric({ value, label }: { value: string; label: string }) {
  return (
    <div>
      <p className="font-semibold text-gray-900 tabular-nums">{value}</p>
      <p className="text-gray-500">{label}</p>
    </div>
  );
}

function ActionButton({
  variant,
  active,
  disabled,
  onClick,
  ariaLabel,
  testid,
}: {
  variant: "approve" | "reject" | "uncertain";
  active: boolean;
  disabled: boolean;
  onClick: (e: React.MouseEvent) => void;
  ariaLabel: string;
  testid: string;
}) {
  const palette = {
    approve: {
      bg: active
        ? "bg-emerald-500 text-white"
        : "bg-emerald-50 text-emerald-700 hover:bg-emerald-100",
      border: "border-emerald-300",
      icon: <CheckIcon />,
    },
    reject: {
      bg: active
        ? "bg-red-500 text-white"
        : "bg-red-50 text-red-700 hover:bg-red-100",
      border: "border-red-300",
      icon: <CrossIcon />,
    },
    uncertain: {
      bg: active
        ? "bg-amber-500 text-white"
        : "bg-amber-50 text-amber-700 hover:bg-amber-100",
      border: "border-amber-300",
      icon: <QuestionIcon />,
    },
  }[variant];

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      aria-pressed={active}
      data-testid={testid}
      className={`inline-flex h-8 w-8 items-center justify-center rounded-full border ${palette.border} ${palette.bg} transition disabled:opacity-50`}
    >
      {palette.icon}
    </button>
  );
}

function Avatar({ name, url }: { name: string; url?: string }) {
  if (url) {
    return (
      <img
        src={url}
        alt={name}
        className="h-12 w-12 shrink-0 rounded-full object-cover"
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
    <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-primary-50 text-sm font-semibold text-primary">
      {initials}
    </div>
  );
}

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${Math.round(n / 100) / 10}K`;
  return String(n);
}

function CheckIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function CrossIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function QuestionIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M9.5 9a2.5 2.5 0 1 1 3.5 2.3c-1 .5-1.5 1-1.5 2.2" />
      <line x1="12" y1="17" x2="12" y2="17.01" />
    </svg>
  );
}
