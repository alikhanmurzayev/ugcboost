import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getCampaign,
  listCampaignApplications,
  replaceCreator,
  sendTzToApproved,
  setApplicationStatus,
} from "@/api/campaigns";
import { campaignKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import SocialLink from "@/features/creatorApplications/components/SocialLink";
import ApplicationsTable, {
  type Column,
} from "@/features/creatorApplications/components/ApplicationsTable";
import CampaignApplicationDrawer from "../components/CampaignApplicationDrawer";
import ReelLightbox from "../components/ReelLightbox";
import ReplacementModal from "../components/ReplacementModal";
import type {
  ApplicationStatus,
  CampaignApplication,
  CreatorReel,
  TzStatus,
} from "../types";

interface Props {
  campaignId: string;
}

export default function SelectedSection({ campaignId }: Props) {
  const { t } = useTranslation("campaigns");
  const queryClient = useQueryClient();
  const [openIndex, setOpenIndex] = useState<number | null>(null);
  const [lightboxReel, setLightboxReel] = useState<CreatorReel | null>(null);
  const [replaceTargetId, setReplaceTargetId] = useState<string | null>(null);

  const campaignQuery = useQuery({
    queryKey: campaignKeys.detail(campaignId),
    queryFn: () => getCampaign(campaignId),
  });
  const applicationsQuery = useQuery({
    queryKey: campaignKeys.applications(campaignId),
    queryFn: () => listCampaignApplications(campaignId),
  });

  function invalidate() {
    queryClient.invalidateQueries({
      queryKey: campaignKeys.applications(campaignId),
    });
  }

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: ApplicationStatus }) =>
      setApplicationStatus(id, status),
    onSuccess: invalidate,
  });
  const sendTzMutation = useMutation({
    mutationFn: () => sendTzToApproved(campaignId),
    onSuccess: invalidate,
  });
  const replaceMutation = useMutation({
    mutationFn: ({ oldId, newId }: { oldId: string; newId: string }) =>
      replaceCreator(oldId, newId),
    onSuccess: invalidate,
  });

  // Approved applications that are still part of the active list (not replaced).
  const visible = useMemo(() => {
    return (applicationsQuery.data ?? []).filter(
      (a) => a.status === "approved" && a.tzStatus !== "replaced",
    );
  }, [applicationsQuery.data]);

  const pendingApplications = useMemo(() => {
    return (applicationsQuery.data ?? []).filter((a) => a.status === "new");
  }, [applicationsQuery.data]);

  const phase = useMemo(() => derivePhase(visible), [visible]);
  const counts = useMemo(() => countTz(visible), [visible]);

  const columns: Column<CampaignApplication>[] = useMemo(
    () =>
      buildColumns(t, phase, {
        onReplace: (id) => setReplaceTargetId(id),
      }),
    [t, phase],
  );

  if (applicationsQuery.isLoading || campaignQuery.isLoading)
    return <Spinner className="mt-12" />;
  if (applicationsQuery.isError)
    return (
      <ErrorState
        message={t("loadError")}
        onRetry={() => void applicationsQuery.refetch()}
      />
    );

  if (visible.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-surface-300 bg-white p-8 text-center text-sm text-gray-500">
        {t("selectedEmpty")}
      </div>
    );
  }

  function setStatus(id: string, status: ApplicationStatus) {
    statusMutation.mutate({ id, status });
  }

  const replaceTarget = replaceTargetId
    ? visible.find((a) => a.id === replaceTargetId) ?? null
    : null;

  return (
    <div data-testid="selected-section">
      {/* Sticky CTA / status banner — visible above the table at all times. */}
      <div className="sticky top-0 z-20 -mx-2 mb-4 bg-surface-50 px-2 py-2">
        <TzBanner
          phase={phase}
          counts={counts}
          target={campaignQuery.data?.creatorsCount ?? visible.length}
          onSendTz={() => sendTzMutation.mutate()}
          isSending={sendTzMutation.isPending}
        />
      </div>

      <div className="rounded-2xl border border-surface-200 bg-white p-5 shadow-sm">
        <div className="mb-4 flex items-baseline justify-between gap-3">
          <h3 className="text-base font-semibold text-gray-900">
            {t("selectedListTitle")}
          </h3>
          <span className="text-sm text-gray-500">
            {t("selectedCount", { count: visible.length })}
          </span>
        </div>

        <ApplicationsTable
          rows={visible}
          columns={columns}
          rowKey={(row) => row.id}
          onRowClick={(row) => {
            const idx = visible.findIndex((a) => a.id === row.id);
            if (idx >= 0) setOpenIndex(idx);
          }}
          emptyMessage={t("selectedEmpty")}
          fixedLayout
          compact
        />
      </div>

      <p className="mt-4 text-xs text-gray-500">{t("selectedHint")}</p>

      {openIndex !== null && (
        <CampaignApplicationDrawer
          applications={visible}
          index={openIndex}
          onClose={() => setOpenIndex(null)}
          onPick={setOpenIndex}
          onAction={(id, status) => {
            setStatus(id, status);
            setOpenIndex(null);
          }}
          onReelClick={setLightboxReel}
          mode="selected"
          // Removing from selected only makes sense before the TZ is sent —
          // afterwards the brand should use the replacement flow instead.
          actionsDisabled={visible[openIndex]?.tzStatus !== "not_sent"}
        />
      )}

      {lightboxReel && (
        <ReelLightbox
          reel={lightboxReel}
          onClose={() => setLightboxReel(null)}
        />
      )}

      {replaceTarget && (
        <ReplacementModal
          declined={replaceTarget}
          candidates={pendingApplications}
          onClose={() => setReplaceTargetId(null)}
          onPick={(newId) => {
            replaceMutation.mutate(
              { oldId: replaceTarget.id, newId },
              { onSuccess: () => setReplaceTargetId(null) },
            );
          }}
          isSubmitting={replaceMutation.isPending}
        />
      )}
    </div>
  );
}

type Phase = "before_send" | "in_progress" | "all_accepted";

interface Counts {
  notSent: number;
  sent: number;
  accepted: number;
  declined: number;
}

function derivePhase(visible: CampaignApplication[]): Phase {
  if (visible.length === 0) return "before_send";
  const allAccepted = visible.every((a) => a.tzStatus === "accepted");
  if (allAccepted) return "all_accepted";
  const allNotSent = visible.every((a) => a.tzStatus === "not_sent");
  if (allNotSent) return "before_send";
  return "in_progress";
}

function countTz(visible: CampaignApplication[]): Counts {
  const c: Counts = { notSent: 0, sent: 0, accepted: 0, declined: 0 };
  for (const a of visible) {
    if (a.tzStatus === "not_sent") c.notSent += 1;
    else if (a.tzStatus === "sent") c.sent += 1;
    else if (a.tzStatus === "accepted") c.accepted += 1;
    else if (a.tzStatus === "declined") c.declined += 1;
  }
  return c;
}

function TzBanner({
  phase,
  counts,
  target,
  onSendTz,
  isSending,
}: {
  phase: Phase;
  counts: Counts;
  target: number;
  onSendTz: () => void;
  isSending: boolean;
}) {
  const { t } = useTranslation("campaigns");
  if (phase === "before_send") {
    const total = counts.notSent;
    const ready = total >= target;
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-primary-200 bg-primary-50 px-5 py-4 shadow-sm">
        <div>
          <p className="text-sm font-semibold text-gray-900">
            {t("tzBanner.beforeSendTitle", { count: total, target })}
          </p>
          <p className="mt-0.5 text-xs text-gray-600">
            {ready
              ? t("tzBanner.beforeSendReadyHint")
              : t("tzBanner.beforeSendShortHint", { needed: target - total })}
          </p>
        </div>
        <button
          type="button"
          onClick={onSendTz}
          disabled={!ready || isSending}
          className="rounded-button bg-primary px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="send-tz-button"
        >
          {isSending ? t("tzBanner.sending") : t("tzBanner.sendButton")}
        </button>
      </div>
    );
  }
  if (phase === "all_accepted") {
    return (
      <div className="rounded-2xl border border-emerald-200 bg-emerald-50 px-5 py-4 shadow-sm">
        <p className="text-sm font-semibold text-emerald-800">
          {t("tzBanner.allAcceptedTitle")}
        </p>
        <p className="mt-0.5 text-xs text-emerald-700">
          {t("tzBanner.allAcceptedHint")}
        </p>
      </div>
    );
  }
  // in_progress
  return (
    <div className="rounded-2xl border border-amber-200 bg-amber-50 px-5 py-4 shadow-sm">
      <p className="text-sm font-semibold text-gray-900">
        {t("tzBanner.inProgressTitle")}
      </p>
      <p className="mt-0.5 text-xs text-gray-700">
        {t("tzBanner.inProgressSummary", {
          accepted: counts.accepted,
          target,
          pending: counts.sent,
          declined: counts.declined,
        })}
      </p>
    </div>
  );
}

function buildColumns(
  t: (key: string, opts?: Record<string, unknown>) => string,
  phase: Phase,
  handlers: {
    onReplace: (id: string) => void;
  },
): Column<CampaignApplication>[] {
  const showStatus = phase !== "before_send";
  const cols: Column<CampaignApplication>[] = [
    {
      key: "index",
      header: "№",
      render: (_row, index) => (
        <span className="text-gray-400 tabular-nums">{index + 1}</span>
      ),
      width: "w-10",
    },
    {
      key: "fullName",
      header: t("columns.fullName"),
      render: (row) => (
        <span className="font-medium text-gray-900">
          {row.creator.lastName} {row.creator.firstName}
        </span>
      ),
      sortValue: (row) =>
        `${row.creator.lastName} ${row.creator.firstName}`.toLowerCase(),
      width: showStatus ? "w-36" : "w-40",
    },
    {
      key: "socials",
      header: t("columns.socials"),
      render: (row) => (
        <div
          className="flex flex-col gap-1"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => e.stopPropagation()}
          role="presentation"
        >
          {row.creator.socials.map((s) => (
            <SocialLink
              key={`${s.platform}-${s.handle}`}
              platform={s.platform}
              handle={s.handle}
              showHandle
            />
          ))}
        </div>
      ),
      width: "w-36",
    },
    {
      key: "followers",
      header: <span title="Подписчики">{t("columns.followers")}</span>,
      render: (row) => (
        <span className="tabular-nums">
          {compact(row.creator.metrics.followers)}
        </span>
      ),
      sortValue: (row) => row.creator.metrics.followers,
      align: "right",
      width: "w-20",
    },
    {
      key: "avgViews",
      header: <span title="Среднее просмотров">{t("columns.avgViews")}</span>,
      render: (row) => (
        <span className="tabular-nums">
          {compact(row.creator.metrics.avgViews)}
        </span>
      ),
      sortValue: (row) => row.creator.metrics.avgViews,
      align: "right",
      width: "w-20",
    },
    {
      key: "er",
      header: <span className="pr-3">{t("columns.er")}</span>,
      render: (row) => (
        <span className="pr-3 tabular-nums">{row.creator.metrics.er}%</span>
      ),
      sortValue: (row) => row.creator.metrics.er,
      align: "right",
      width: "w-16",
    },
    {
      key: "city",
      header: t("columns.city"),
      render: (row) => row.creator.city.name,
      sortValue: (row) => row.creator.city.name.toLowerCase(),
      width: "w-24",
    },
  ];
  if (showStatus) {
    cols.push({
      key: "tzStatus",
      header: t("columns.tzStatus"),
      render: (row) => (
        <div
          className="flex flex-wrap items-center gap-2"
          onClick={(e) => {
            if (row.tzStatus === "declined") e.stopPropagation();
          }}
          onKeyDown={(e) => {
            if (row.tzStatus === "declined") e.stopPropagation();
          }}
          role="presentation"
        >
          <TzStatusPill status={row.tzStatus} />
          {row.tzStatus === "declined" && (
            <button
              type="button"
              onClick={() => handlers.onReplace(row.id)}
              className="rounded-button bg-primary px-2.5 py-1 text-xs font-semibold text-white hover:bg-primary-600"
              data-testid={`replace-${row.id}`}
            >
              {t("tzActions.replace")}
            </button>
          )}
        </div>
      ),
      width: "w-44",
    });
  }
  return cols;
}

function TzStatusPill({ status }: { status: TzStatus }) {
  const { t } = useTranslation("campaigns");
  const map: Record<TzStatus, { cls: string; icon: string }> = {
    not_sent: { cls: "bg-surface-200 text-gray-600", icon: "—" },
    sent: { cls: "bg-amber-100 text-amber-800", icon: "🟡" },
    accepted: { cls: "bg-emerald-100 text-emerald-800", icon: "✅" },
    declined: { cls: "bg-red-100 text-red-800", icon: "❌" },
    replaced: { cls: "bg-surface-100 text-gray-400", icon: "↻" },
  };
  const { cls, icon } = map[status];
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      <span aria-hidden>{icon}</span>
      {t(`tzStatus.${status}`)}
    </span>
  );
}

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${Math.round(n / 100) / 10}K`;
  return String(n);
}
