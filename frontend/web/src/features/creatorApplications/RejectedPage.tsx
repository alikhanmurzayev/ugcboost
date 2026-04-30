import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getApplication, listApplications } from "@/api/creatorApplications";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import ApplicationsTable, { type Column } from "./components/ApplicationsTable";
import ApplicationDrawer from "./components/ApplicationDrawer";
import ApplicationFilters from "./components/ApplicationFilters";
import InternalNoteField from "./components/InternalNoteField";
import { CategoryChips } from "./components/CategoryChip";
import QualityIndicatorDot from "./components/QualityIndicatorDot";
import SocialLink from "./components/SocialLink";
import { applyFilters, isFilterActive, parseFilters } from "./filters";
import { sortApplications, type SortState } from "./sort";
import type { Application } from "./types";

export default function RejectedPage() {
  const { t } = useTranslation("creatorApplications");
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("id");
  const [sort, setSort] = useState<SortState>({
    key: "fullName",
    dir: "asc",
  });

  const listQuery = useQuery({
    queryKey: creatorApplicationKeys.list("rejected"),
    queryFn: () => listApplications("rejected"),
  });

  const detailQuery = useQuery({
    queryKey: creatorApplicationKeys.detail(selectedId ?? ""),
    queryFn: () => getApplication(selectedId ?? ""),
    enabled: !!selectedId,
  });

  const columns: Column<Application>[] = useMemo(() => buildColumns(t), [t]);

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
    <div data-testid="creator-applications-rejected-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("stages.rejected.title")}
        {!listQuery.isLoading && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="rejected-total"
          >
            {totalCount}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">
        {t("stages.rejected.description")}
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
          <ApplicationFilters />
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
        {detailQuery.data && (
          <InternalNoteField
            key={detailQuery.data.id}
            application={detailQuery.data}
          />
        )}
      </ApplicationDrawer>
    </div>
  );
}

function buildColumns(t: (key: string) => string): Column<Application>[] {
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
      width: "w-48",
    },
    {
      key: "qualityIndicator",
      header: t("columns.qualityIndicator"),
      render: (row) => <QualityIndicatorDot value={row.qualityIndicator} />,
      sortValue: (row) => qualityIndicatorOrder(row.qualityIndicator),
      width: "w-24",
    },
    {
      key: "rejectionComment",
      header: t("columns.rejectionComment"),
      render: (row) =>
        row.rejectionComment ? (
          <span
            className="line-clamp-2 text-gray-600"
            title={row.rejectionComment}
          >
            {row.rejectionComment}
          </span>
        ) : (
          <span className="text-gray-400">—</span>
        ),
    },
    {
      key: "internalNote",
      header: t("drawer.internalNote"),
      render: (row) =>
        row.internalNote ? (
          <span
            className="line-clamp-2 text-gray-600"
            title={row.internalNote}
          >
            {row.internalNote}
          </span>
        ) : (
          <span className="text-gray-400">—</span>
        ),
    },
  ];
}

function qualityIndicatorOrder(value?: string): number {
  switch (value) {
    case "green":
      return 0;
    case "orange":
      return 1;
    case "red":
      return 2;
    default:
      return 3;
  }
}

