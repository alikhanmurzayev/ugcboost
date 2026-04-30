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
import { CategoryChips } from "./components/CategoryChip";
import SocialLink from "./components/SocialLink";
import { applyFilters, isFilterActive, parseFilters } from "./filters";
import { sortApplications, type SortState } from "./sort";
import type { Application } from "./types";

export default function CreatorsPage() {
  const { t } = useTranslation(["creators", "creatorApplications"]);
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("id");
  const [sort, setSort] = useState<SortState>({ key: "fullName", dir: "asc" });

  const listQuery = useQuery({
    queryKey: creatorApplicationKeys.list("creators"),
    queryFn: () => listApplications("creators"),
  });

  const detailQuery = useQuery({
    queryKey: creatorApplicationKeys.detail(selectedId ?? ""),
    queryFn: () => getApplication(selectedId ?? ""),
    enabled: !!selectedId,
  });

  const columns: Column<Application>[] = useMemo(
    () => buildColumns(t),
    [t],
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

  const totalCount = listQuery.data?.length ?? 0;

  return (
    <div data-testid="creators-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("creators:title")}
        {!listQuery.isLoading && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="creators-total"
          >
            {totalCount}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">{t("creators:description")}</p>

      {listQuery.isLoading ? (
        <Spinner className="mt-6" />
      ) : listQuery.isError ? (
        <ErrorState
          message={t("creatorApplications:loadError")}
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
                ? t("creatorApplications:emptyFiltered")
                : t("creators:empty")
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
      />
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
      header: t("creatorApplications:columns.fullName"),
      render: (row) => (
        <span className="font-medium text-gray-900">
          {row.lastName} {row.firstName}
        </span>
      ),
      sortValue: (row) => `${row.lastName} ${row.firstName}`.toLowerCase(),
    },
    {
      key: "socials",
      header: t("creatorApplications:columns.socials"),
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
      header: t("creatorApplications:columns.categories"),
      render: (row) => <CategoryChips categories={row.categories} />,
    },
    {
      key: "age",
      header: t("creatorApplications:columns.age"),
      render: (row) => calcAge(row.birthDate),
      sortValue: (row) => calcAge(row.birthDate),
      width: "w-16",
    },
    {
      key: "city",
      header: t("creatorApplications:columns.city"),
      render: (row) => row.city.name,
      sortValue: (row) => row.city.name.toLowerCase(),
      width: "w-28",
    },
    {
      key: "rating",
      header: t("creators:columns.rating"),
      render: (row) =>
        row.rating !== undefined ? (
          <span className="inline-flex items-center gap-1">
            <StarIcon />
            <span>{row.rating.toFixed(1)}</span>
          </span>
        ) : (
          "—"
        ),
      sortValue: (row) => row.rating ?? -1,
      align: "right",
      width: "w-20",
    },
    {
      key: "completedOrders",
      header: t("creators:columns.completedOrders"),
      render: (row) => row.completedOrders ?? 0,
      sortValue: (row) => row.completedOrders ?? 0,
      align: "right",
      width: "w-20",
    },
    {
      key: "activeOrders",
      header: t("creators:columns.activeOrders"),
      render: (row) => row.activeOrders ?? 0,
      sortValue: (row) => row.activeOrders ?? 0,
      align: "right",
      width: "w-20",
    },
  ];
}

function StarIcon() {
  return (
    <svg
      className="h-3.5 w-3.5 text-amber-500"
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
    </svg>
  );
}

function calcAge(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}
