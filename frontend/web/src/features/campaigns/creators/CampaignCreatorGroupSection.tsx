import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  CampaignCreatorStatus,
  CampaignNotifyUndelivered,
} from "@/api/campaignCreators";
import CampaignCreatorsTable, {
  type SelectAllState,
} from "./CampaignCreatorsTable";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";
import {
  MAX_NOTIFY_BATCH,
  type SectionResult,
  UNDELIVERED_REASON_KEY,
} from "./notifyResult";

interface ValidationDetail {
  creatorId: string;
  currentStatus: CampaignCreatorStatus;
}

interface CampaignCreatorGroupSectionProps {
  status: CampaignCreatorStatus;
  title: string;
  rows: CampaignCreatorRow[];
  actionLabel?: string;
  actionSubmittingLabel?: string;
  onSubmit?: (
    creatorIds: string[],
    namesSnapshot: Record<string, string>,
  ) => void;
  result?: SectionResult | null;
  isSubmitting: boolean;
  isPending: boolean;
  onRemove?: (row: CampaignCreatorRow) => void;
  drawerSelectedCreatorId?: string;
  onRowClick: (row: CampaignCreatorRow) => void;
  onToggleTicketSent?: (creatorId: string, next: boolean) => void;
  ticketSentPendingCreatorId?: string;
}

export default function CampaignCreatorGroupSection({
  status,
  title,
  rows,
  actionLabel,
  actionSubmittingLabel,
  onSubmit,
  result,
  isSubmitting,
  isPending,
  onRemove,
  drawerSelectedCreatorId,
  onRowClick,
  onToggleTicketSent,
  ticketSentPendingCreatorId,
}: CampaignCreatorGroupSectionProps) {
  const { t } = useTranslation("campaigns");
  const headingId = `campaign-creators-group-heading-${status}`;

  const [checkedCreatorIds, setCheckedCreatorIds] = useState<Set<string>>(
    () => new Set(),
  );

  const rowIds = useMemo(
    () => new Set(rows.map((r) => r.campaignCreator.creatorId)),
    [rows],
  );

  const effectiveCheckedIds = useMemo(() => {
    const next = new Set<string>();
    for (const id of checkedCreatorIds) {
      if (rowIds.has(id)) next.add(id);
    }
    return next;
  }, [checkedCreatorIds, rowIds]);

  const size = effectiveCheckedIds.size;
  const cappedRowsLength = Math.min(rows.length, MAX_NOTIFY_BATCH);

  const selectAllState = useMemo<SelectAllState>(() => {
    if (size === 0) return "unchecked";
    if (size < cappedRowsLength) return "indeterminate";
    return "checked";
  }, [size, cappedRowsLength]);

  // When the parent flips submitting from true → false, the mutation just
  // settled — clear the selection so the next click starts from a clean slate.
  const prevSubmittingRef = useRef(isSubmitting);
  useEffect(() => {
    if (prevSubmittingRef.current && !isSubmitting) {
      setCheckedCreatorIds(new Set());
    }
    prevSubmittingRef.current = isSubmitting;
  }, [isSubmitting]);

  function toggleOne(creatorId: string) {
    setCheckedCreatorIds((prev) => {
      const next = new Set(prev);
      if (next.has(creatorId)) {
        next.delete(creatorId);
        return next;
      }
      if (next.size >= MAX_NOTIFY_BATCH) return prev;
      next.add(creatorId);
      return next;
    });
  }

  function toggleAll() {
    setCheckedCreatorIds((prev) => {
      const allIds = rows.map((r) => r.campaignCreator.creatorId);
      const everyChecked =
        allIds.length > 0 && allIds.slice(0, MAX_NOTIFY_BATCH).every((id) => prev.has(id));
      if (everyChecked) return new Set();
      return new Set(allIds.slice(0, MAX_NOTIFY_BATCH));
    });
  }

  const isBusy = isPending || isSubmitting;
  const hasAction = !!onSubmit && !!actionLabel;
  const isAtCap = size >= MAX_NOTIFY_BATCH;
  const isOverCap = rows.length > MAX_NOTIFY_BATCH;
  const submitDisabled = !hasAction || size === 0 || isBusy;
  const buttonLabel =
    isBusy && actionSubmittingLabel ? actionSubmittingLabel : actionLabel;

  function handleSubmit() {
    if (!onSubmit) return;
    if (submitDisabled) return;
    const creatorIds = [...effectiveCheckedIds];
    const namesSnapshot = snapshotNames(creatorIds, rows, t);
    onSubmit(creatorIds, namesSnapshot);
  }

  return (
    <section
      className="mt-6 rounded-card border border-surface-300 bg-surface-50 p-4"
      data-testid={`campaign-creators-group-${status}`}
      aria-labelledby={headingId}
    >
      <div className="flex items-center justify-between">
        <h3
          id={headingId}
          className="flex items-baseline gap-3 text-base font-semibold text-gray-900"
        >
          {title}
          <span className="text-sm font-medium text-gray-400">
            {rows.length}
          </span>
        </h3>
        {hasAction ? (
          <button
            type="button"
            onClick={handleSubmit}
            disabled={submitDisabled}
            data-testid={`campaign-creators-group-action-${status}`}
            className="rounded-button bg-primary px-3 py-1.5 text-sm font-medium text-white transition hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {buttonLabel}
          </button>
        ) : null}
      </div>

      {hasAction && (size > 0 || isOverCap) && (
        <div className="mt-2 flex items-center gap-3 text-xs text-gray-500">
          <span data-testid={`campaign-creators-group-counter-${status}`}>
            {t("campaignCreators.selectionCounter", {
              count: size,
              max: MAX_NOTIFY_BATCH,
            })}
          </span>
          {(isAtCap || isOverCap) && (
            <span
              className="text-amber-700"
              data-testid={`campaign-creators-group-cap-hint-${status}`}
            >
              {t("campaignCreators.capHint")}
            </span>
          )}
        </div>
      )}

      {result && (
        <ResultBlock
          status={status}
          result={result}
          t={t}
          testidBase={`campaign-creators-group-result-${status}`}
        />
      )}

      {rows.length > 0 ? (
        <CampaignCreatorsTable
          rows={rows}
          status={status}
          selectedKey={drawerSelectedCreatorId}
          onRowClick={onRowClick}
          onRemove={onRemove}
          emptyMessage=""
          checkedCreatorIds={hasAction ? effectiveCheckedIds : undefined}
          onToggleOne={hasAction ? toggleOne : undefined}
          onToggleAll={hasAction ? toggleAll : undefined}
          selectAllState={selectAllState}
          selectAllTestId={`campaign-creators-select-all-${status}`}
          rowSelectionDisabled={isBusy}
          onToggleTicketSent={onToggleTicketSent}
          ticketSentPendingCreatorId={ticketSentPendingCreatorId}
        />
      ) : (
        <p
          className="mt-4 text-sm text-gray-400"
          data-testid={`campaign-creators-group-empty-${status}`}
        >
          {t("campaignCreators.emptyGroup")}
        </p>
      )}
    </section>
  );
}

interface ResultBlockProps {
  status: CampaignCreatorStatus;
  result: SectionResult;
  t: (key: string, opts?: Record<string, unknown>) => string;
  testidBase: string;
}

function ResultBlock({ status, result, t, testidBase }: ResultBlockProps) {
  if (result.kind === "validation_error") {
    const details = result.validationDetails ?? [];
    const names = result.detailNames ?? {};
    return (
      <div
        className="mt-3 rounded-button border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800"
        role="alert"
        data-testid={`${testidBase}-validation`}
      >
        <p>{t("campaignCreators.result.validationError")}</p>
        {details.length > 0 && (
          <ul
            className="mt-1 list-inside list-disc space-y-1"
            data-testid={`campaign-creators-group-validation-details-${status}`}
          >
            {details.map((d) => (
              <li
                key={d.creatorId}
                data-testid={`campaign-creators-group-validation-details-${status}-${d.creatorId}`}
              >
                {names[d.creatorId] ?? t("campaignCreators.deletedPlaceholder")}{" "}
                → {t(currentStatusKey(d.currentStatus))}
              </li>
            ))}
          </ul>
        )}
      </div>
    );
  }

  if (result.kind === "contract_template_required") {
    return (
      <p
        className="mt-3 rounded-button border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800"
        role="alert"
        data-testid={`${testidBase}-contract-template-required`}
      >
        {t("campaignCreators.result.contractTemplateRequired")}
      </p>
    );
  }

  if (result.kind === "validation_unknown") {
    return (
      <p
        className="mt-3 rounded-button border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800"
        role="alert"
        data-testid={`${testidBase}-validation-unknown`}
      >
        {t("campaignCreators.result.validationUnknown")}
      </p>
    );
  }

  if (result.kind === "network_error") {
    return (
      <p
        className="mt-3 rounded-button border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
        role="alert"
        data-testid={`${testidBase}-network`}
      >
        {t("campaignCreators.result.networkError")}
      </p>
    );
  }

  const undelivered: CampaignNotifyUndelivered[] = result.undelivered ?? [];
  const delivered = result.deliveredCount ?? 0;
  const names = result.undeliveredNames ?? {};
  return (
    <div
      className="mt-3 rounded-button border border-surface-300 bg-white px-3 py-2 text-sm text-gray-800"
      role="status"
      data-testid={`${testidBase}-success`}
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
                {names[u.creatorId] ?? t("campaignCreators.deletedPlaceholder")}
                :{" "}
                {t(
                  UNDELIVERED_REASON_KEY[u.reason] ??
                    UNDELIVERED_REASON_KEY.unknown,
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

function currentStatusKey(s: CampaignCreatorStatus): string {
  return `campaignCreators.currentStatus.${s}`;
}

export type { ValidationDetail };

function snapshotNames(
  creatorIds: string[],
  rows: CampaignCreatorRow[],
  t: (key: string) => string,
): Record<string, string> {
  const placeholder = t("campaignCreators.deletedPlaceholder");
  const map: Record<string, string> = {};
  for (const id of creatorIds) {
    const row = rows.find((r) => r.campaignCreator.creatorId === id);
    map[id] = row?.creator
      ? `${row.creator.lastName} ${row.creator.firstName}`
      : placeholder;
  }
  return map;
}
