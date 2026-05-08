import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient, type UseMutationResult } from "@tanstack/react-query";
import { ApiError } from "@/api/client";
import type {
  CampaignCreatorStatus,
  CampaignNotifyResult,
  CampaignNotifyUndelivered,
} from "@/api/campaignCreators";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";
import CampaignCreatorsTable, {
  type SelectAllState,
} from "./CampaignCreatorsTable";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

type ResultKind = "success" | "validation_error" | "network_error";

interface SectionResult {
  kind: ResultKind;
  undelivered?: CampaignNotifyUndelivered[];
  deliveredCount?: number;
}

interface CampaignCreatorGroupSectionProps {
  status: CampaignCreatorStatus;
  campaignId: string;
  title: string;
  rows: CampaignCreatorRow[];
  actionLabel?: string;
  mutation?: UseMutationResult<CampaignNotifyResult, ApiError, string[]>;
  onRemove: (row: CampaignCreatorRow) => void;
  drawerSelectedCreatorId?: string;
  onRowClick: (row: CampaignCreatorRow) => void;
}

export default function CampaignCreatorGroupSection({
  status,
  campaignId,
  title,
  rows,
  actionLabel,
  mutation,
  onRemove,
  drawerSelectedCreatorId,
  onRowClick,
}: CampaignCreatorGroupSectionProps) {
  const { t } = useTranslation("campaigns");
  const queryClient = useQueryClient();

  const [checkedCreatorIds, setCheckedCreatorIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [result, setResult] = useState<SectionResult | null>(null);

  const size = checkedCreatorIds.size;

  const selectAllState = useMemo<SelectAllState>(() => {
    if (size === 0) return "unchecked";
    if (size < rows.length) return "indeterminate";
    return "checked";
  }, [size, rows.length]);

  function toggleOne(creatorId: string) {
    setCheckedCreatorIds((prev) => {
      const next = new Set(prev);
      if (next.has(creatorId)) {
        next.delete(creatorId);
      } else {
        next.add(creatorId);
      }
      return next;
    });
  }

  function toggleAll() {
    setCheckedCreatorIds((prev) => {
      if (prev.size === rows.length) return new Set();
      return new Set(rows.map((r) => r.campaignCreator.creatorId));
    });
  }

  const isPending = mutation?.isPending ?? false;
  const submitDisabled =
    !mutation || !actionLabel || size === 0 || isPending || isSubmitting;

  function handleSubmit() {
    if (!mutation || !actionLabel) return;
    if (submitDisabled) return;
    const creatorIds = [...checkedCreatorIds];
    setIsSubmitting(true);
    setResult(null);
    mutation.mutate(creatorIds, {
      onSettled: (data, error) => {
        void queryClient.invalidateQueries({
          queryKey: campaignCreatorKeys.list(campaignId),
        });
        setCheckedCreatorIds(new Set());
        setIsSubmitting(false);
        setResult(parseSettled(data, error, creatorIds.length));
      },
    });
  }

  return (
    <section
      className="mt-6 rounded-card border border-surface-300 bg-white p-4"
      data-testid={`campaign-creators-group-${status}`}
    >
      <div className="flex items-center justify-between">
        <h3 className="flex items-baseline gap-3 text-base font-semibold text-gray-900">
          {title}
          <span className="text-sm font-medium text-gray-400">
            {rows.length}
          </span>
        </h3>
        {actionLabel && mutation ? (
          <button
            type="button"
            onClick={handleSubmit}
            disabled={submitDisabled}
            data-testid={`campaign-creators-group-action-${status}`}
            className="rounded-button bg-primary px-3 py-1.5 text-sm font-medium text-white transition hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {actionLabel}
          </button>
        ) : null}
      </div>

      {result && (
        <ResultBlock
          status={status}
          result={result}
          rows={rows}
          t={t}
          data-testid={`campaign-creators-group-result-${status}`}
        />
      )}

      <CampaignCreatorsTable
        rows={rows}
        selectedKey={drawerSelectedCreatorId}
        onRowClick={onRowClick}
        onRemove={onRemove}
        emptyMessage=""
        checkedCreatorIds={mutation && actionLabel ? checkedCreatorIds : undefined}
        onToggleOne={mutation && actionLabel ? toggleOne : undefined}
        onToggleAll={mutation && actionLabel ? toggleAll : undefined}
        selectAllState={selectAllState}
        selectAllTestId={`campaign-creators-select-all-${status}`}
      />
    </section>
  );
}

function parseSettled(
  data: CampaignNotifyResult | undefined,
  error: ApiError | null,
  attempted: number,
): SectionResult {
  if (
    error instanceof ApiError &&
    error.status === 422 &&
    error.code === "CAMPAIGN_CREATOR_BATCH_INVALID"
  ) {
    return { kind: "validation_error" };
  }
  if (error) {
    return { kind: "network_error" };
  }
  if (data) {
    const undelivered = data.data.undelivered;
    return {
      kind: "success",
      undelivered,
      deliveredCount: attempted - undelivered.length,
    };
  }
  return { kind: "network_error" };
}

interface ResultBlockProps {
  status: CampaignCreatorStatus;
  result: SectionResult;
  rows: CampaignCreatorRow[];
  t: (key: string, opts?: Record<string, unknown>) => string;
  "data-testid": string;
}

function ResultBlock({ status, result, rows, t, ...rest }: ResultBlockProps) {
  if (result.kind === "validation_error") {
    return (
      <p
        className="mt-3 rounded-button border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800"
        role="alert"
        data-testid={rest["data-testid"]}
      >
        {t("campaignCreators.result.validationError")}
      </p>
    );
  }

  if (result.kind === "network_error") {
    return (
      <p
        className="mt-3 rounded-button border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
        role="alert"
        data-testid={rest["data-testid"]}
      >
        {t("campaignCreators.result.networkError")}
      </p>
    );
  }

  const undelivered = result.undelivered ?? [];
  const delivered = result.deliveredCount ?? 0;
  return (
    <div
      className="mt-3 rounded-button border border-surface-300 bg-surface-100 px-3 py-2 text-sm text-gray-800"
      data-testid={rest["data-testid"]}
    >
      <p>{t("campaignCreators.result.delivered", { count: delivered })}</p>
      {undelivered.length > 0 && (
        <>
          <p className="mt-2">
            {t("campaignCreators.result.undelivered", {
              count: undelivered.length,
            })}
          </p>
          <ul
            className="mt-1 list-inside list-disc space-y-1"
            data-testid={`campaign-creators-group-undelivered-${status}`}
          >
            {undelivered.map((u) => (
              <li
                key={u.creatorId}
                data-testid={`campaign-creators-group-undelivered-${status}-${u.creatorId}`}
              >
                {undeliveredName(u.creatorId, rows, t)}:{" "}
                {t(`campaignCreators.undeliveredReason.${u.reason}`, {
                  defaultValue: t("campaignCreators.undeliveredReason.unknown"),
                })}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

function undeliveredName(
  creatorId: string,
  rows: CampaignCreatorRow[],
  t: (key: string) => string,
): string {
  const row = rows.find((r) => r.campaignCreator.creatorId === creatorId);
  if (row?.creator) {
    return `${row.creator.lastName} ${row.creator.firstName}`;
  }
  return t("campaignCreators.deletedPlaceholder");
}
