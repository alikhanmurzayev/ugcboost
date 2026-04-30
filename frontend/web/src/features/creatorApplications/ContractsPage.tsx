import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getApplication,
  listApplications,
  sendContracts,
} from "@/api/creatorApplications";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import ApplicationsTable, { type Column } from "./components/ApplicationsTable";
import ApplicationDrawer from "./components/ApplicationDrawer";
import ApplicationFilters from "./components/ApplicationFilters";
import { CategoryChips } from "./components/CategoryChip";
import ContractsActions from "./components/ContractsActions";
import HoursBadge from "./components/HoursBadge";
import { hoursSince } from "./hours";
import SocialLink from "./components/SocialLink";
import { applyFilters, isFilterActive, parseFilters } from "./filters";
import { sortApplications, type SortState } from "./sort";
import type { Application, ContractStatus } from "./types";

export default function ContractsPage() {
  const { t } = useTranslation("creatorApplications");
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("id");
  const [sort, setSort] = useState<SortState>({
    key: "hoursInStage",
    dir: "desc",
  });
  const [selected, setSelected] = useState<string[]>([]);
  const [bulkError, setBulkError] = useState("");

  const listQuery = useQuery({
    queryKey: creatorApplicationKeys.list("contracts"),
    queryFn: () => listApplications("contracts"),
  });

  const detailQuery = useQuery({
    queryKey: creatorApplicationKeys.detail(selectedId ?? ""),
    queryFn: () => getApplication(selectedId ?? ""),
    enabled: !!selectedId,
  });

  const sendableIds = useMemo(
    () =>
      (listQuery.data ?? [])
        .filter((a) => a.contractStatus === "not_sent")
        .map((a) => a.id),
    [listQuery.data],
  );

  // Drop selections whose application is no longer sendable (e.g. its status
  // moved to "sent" after a successful bulk action). Without this, the action
  // bar can keep showing "Разослать (1)" pointing at an already-sent record.
  const effectiveSelected = useMemo(
    () => selected.filter((id) => sendableIds.includes(id)),
    [selected, sendableIds],
  );

  function toggleOne(id: string) {
    if (!sendableIds.includes(id)) return;
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((i) => i !== id) : [...prev, id],
    );
  }

  function toggleAll() {
    if (
      effectiveSelected.length === sendableIds.length &&
      sendableIds.length > 0
    ) {
      setSelected([]);
    } else {
      setSelected(sendableIds);
    }
  }

  const sendMut = useMutation({
    mutationFn: (ids: string[]) => sendContracts(ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
      setSelected([]);
    },
    onError(err) {
      setBulkError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.sendContractsError"),
      );
    },
  });

  const columns: Column<Application>[] = useMemo(
    () =>
      buildColumns(
        t,
        effectiveSelected,
        toggleOne,
        toggleAll,
        sendableIds.length,
        effectiveSelected.length === sendableIds.length &&
          sendableIds.length > 0,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, effectiveSelected, sendableIds.length],
  );

  const filtered = useMemo(
    () => applyFilters(listQuery.data ?? [], parseFilters(searchParams)),
    [listQuery.data, searchParams],
  );
  const sortedRows = useMemo(
    () => sortApplications(filtered, sort, columns),
    [filtered, sort, columns],
  );

  const currentIdx = sortedRows.findIndex((r) => r.id === selectedId);
  const canPrev = currentIdx > 0;
  const canNext = currentIdx >= 0 && currentIdx < sortedRows.length - 1;
  const totalCount = listQuery.data?.length ?? 0;

  function openApplication(id: string) {
    const next = new URLSearchParams(searchParams);
    next.set("id", id);
    setSearchParams(next);
  }

  function closeApplication() {
    const next = new URLSearchParams(searchParams);
    next.delete("id");
    setSearchParams(next);
  }

  function goPrev() {
    const prev = sortedRows[currentIdx - 1];
    if (prev) openApplication(prev.id);
  }

  function goNext() {
    const next = sortedRows[currentIdx + 1];
    if (next) openApplication(next.id);
  }

  return (
    <div data-testid="creator-applications-contracts-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("stages.contracts.title")}
        {!listQuery.isLoading && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="contracts-total"
          >
            {totalCount}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">
        {t("stages.contracts.description")}
      </p>

      {listQuery.isLoading ? (
        <Spinner className="mt-6" />
      ) : listQuery.isError ? (
        <ErrorState
          message={t("loadError")}
          onRetry={() => void listQuery.refetch()}
        />
      ) : (
        <>
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <ApplicationFilters />
            {effectiveSelected.length > 0 && (
              <div
                className="flex items-center gap-3 rounded-card bg-primary-50 px-3 py-1.5"
                data-testid="bulk-action-bar"
              >
                <span className="text-sm text-gray-700">
                  {t("actions.selectedCount", {
                    count: effectiveSelected.length,
                  })}
                </span>
                <button
                  type="button"
                  onClick={() => sendMut.mutate(effectiveSelected)}
                  disabled={sendMut.isPending}
                  className="rounded-button bg-primary px-4 py-1.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
                  data-testid="send-contracts-button"
                >
                  {sendMut.isPending
                    ? t("actions.sending")
                    : `${t("actions.sendContracts")} (${effectiveSelected.length})`}
                </button>
              </div>
            )}
          </div>
          {bulkError && (
            <p className="mt-2 text-sm text-red-600" role="alert">
              {bulkError}
            </p>
          )}
          <ApplicationsTable
            rows={sortedRows}
            columns={columns}
            rowKey={(row) => row.id}
            sort={sort}
            onSortChange={setSort}
            onRowClick={(row) => openApplication(row.id)}
            selectedKey={selectedId ?? undefined}
            emptyMessage={
              isFilterActive(parseFilters(searchParams))
                ? t("emptyFiltered")
                : t("empty")
            }
          />
        </>
      )}

      <ApplicationDrawer
        application={detailQuery.data}
        isLoading={detailQuery.isLoading}
        open={!!selectedId}
        onClose={closeApplication}
        onPrev={goPrev}
        onNext={goNext}
        canPrev={canPrev}
        canNext={canNext}
      >
        {detailQuery.data && <ContractsActions application={detailQuery.data} />}
      </ApplicationDrawer>
    </div>
  );
}

function buildColumns(
  t: (key: string, opts?: Record<string, unknown>) => string,
  selected: string[],
  toggleOne: (id: string) => void,
  toggleAll: () => void,
  sendableCount: number,
  allSelected: boolean,
): Column<Application>[] {
  return [
    {
      key: "index",
      header: "№",
      render: (_row, index) => (
        <span className="text-gray-400">{index + 1}</span>
      ),
      width: "w-10",
    },
    {
      key: "select",
      header: (
        <input
          type="checkbox"
          checked={allSelected}
          disabled={sendableCount === 0}
          onChange={toggleAll}
          onClick={(e) => e.stopPropagation()}
          aria-label="Выбрать все доступные"
          className="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary focus:ring-primary disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="bulk-select-all"
        />
      ),
      render: (row) => {
        const sendable = row.contractStatus === "not_sent";
        return (
          <input
            type="checkbox"
            checked={selected.includes(row.id)}
            disabled={!sendable}
            onChange={() => toggleOne(row.id)}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => e.stopPropagation()}
            aria-label={
              sendable
                ? "Выбрать заявку"
                : "Договор уже отправлен — выбор недоступен"
            }
            className="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary focus:ring-primary disabled:cursor-not-allowed disabled:opacity-50"
            data-testid={`bulk-select-${row.id}`}
          />
        );
      },
      width: "w-10",
    },
    {
      key: "fullName",
      header: t("columns.fullName"),
      render: (row) => (
        <span className="font-medium text-gray-900">
          {row.lastName} {row.firstName}
        </span>
      ),
      sortValue: (row) => `${row.lastName} ${row.firstName}`.toLowerCase(),
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
          {row.socials.map((s) => (
            <SocialLink
              key={`${s.platform}-${s.handle}`}
              platform={s.platform}
              handle={s.handle}
              showHandle
            />
          ))}
        </div>
      ),
      width: "w-44",
    },
    {
      key: "categories",
      header: t("columns.categories"),
      render: (row) => <CategoryChips categories={row.categories} />,
    },
    {
      key: "age",
      header: t("columns.age"),
      render: (row) => calcAge(row.birthDate),
      sortValue: (row) => calcAge(row.birthDate),
      width: "w-14",
    },
    {
      key: "city",
      header: t("columns.city"),
      render: (row) => row.city.name,
      sortValue: (row) => row.city.name.toLowerCase(),
      width: "w-24",
    },
    {
      key: "contractStatus",
      header: t("columns.contractStatus"),
      render: (row) => <StatusPill status={row.contractStatus} />,
      sortValue: (row) => statusOrder(row.contractStatus),
      width: "w-32",
    },
    {
      key: "hoursInStage",
      header: t("columns.hoursInStage"),
      render: (row) => {
        const since = waitingSince(row);
        if (!since) return "—";
        return <HoursBadge hours={hoursSince(since)} />;
      },
      sortValue: (row) => {
        const since = waitingSince(row);
        return since ? hoursSince(since) : 0;
      },
      align: "right",
      width: "w-24",
    },
  ];
}

function waitingSince(row: Application): string | undefined {
  return row.contractStatus === "sent" ? row.updatedAt : row.approvedAt;
}

function StatusPill({ status }: { status?: ContractStatus }) {
  const { t } = useTranslation("creatorApplications");
  const variant =
    status === "signed"
      ? "bg-emerald-100 text-emerald-800"
      : status === "sent"
        ? "bg-sky-100 text-sky-800"
        : "bg-amber-100 text-amber-800";
  const key = status ?? "not_sent";
  return (
    <span
      className={`inline-flex whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${variant}`}
    >
      {t(`contractStatus.${key}`)}
    </span>
  );
}

function statusOrder(status?: ContractStatus): number {
  switch (status) {
    case "not_sent":
      return 0;
    case "sent":
      return 1;
    case "signed":
      return 2;
    default:
      return 3;
  }
}

function calcAge(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}
