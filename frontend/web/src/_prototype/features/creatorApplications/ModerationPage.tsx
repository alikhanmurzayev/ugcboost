import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getApplication, listApplications } from "@/_prototype/api/creatorApplications";
import { creatorApplicationKeys } from "@/_prototype/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import ApplicationsTable, { type Column } from "./components/ApplicationsTable";
import ApplicationDrawer from "./components/ApplicationDrawer";
import ApplicationFilters from "./components/ApplicationFilters";
import ModerationActions from "./components/ModerationActions";
import { CategoryChips } from "./components/CategoryChip";
import HoursBadge from "./components/HoursBadge";
import { hoursSince } from "./hours";
import QualityIndicatorDot from "./components/QualityIndicatorDot";
import SocialLink from "./components/SocialLink";
import { applyFilters, isFilterActive, parseFilters } from "./filters";
import { sortApplications, type SortState } from "./sort";
import type { Application } from "./types";

export default function ModerationPage() {
  const { t } = useTranslation("prototype_creatorApplications");
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("id");
  const [sort, setSort] = useState<SortState>({
    key: "submittedAt",
    dir: "desc",
  });

  const listQuery = useQuery({
    queryKey: creatorApplicationKeys.list("moderation"),
    queryFn: () => listApplications("moderation"),
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
    <div data-testid="creator-applications-moderation-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("stages.moderation.title")}
        {!listQuery.isLoading && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="moderation-total"
          >
            {totalCount}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">
        {t("stages.moderation.description")}
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
          <ApplicationFilters searchOnly />
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
        {detailQuery.data && <ModerationActions application={detailQuery.data} />}
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
      key: "qualityIndicator",
      header: t("columns.qualityIndicator"),
      render: (row) => <QualityIndicatorDot value={row.qualityIndicator} />,
      sortValue: (row) => qualityIndicatorOrder(row.qualityIndicator),
      width: "w-24",
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
      width: "w-20",
    },
    {
      key: "city",
      header: t("columns.city"),
      render: (row) => row.city.name,
      sortValue: (row) => row.city.name.toLowerCase(),
      width: "w-32",
    },
    {
      key: "waiting",
      header: t("columns.waiting"),
      render: (row) => <HoursBadge hours={hoursSince(row.updatedAt)} />,
      sortValue: (row) => hoursSince(row.updatedAt),
      align: "right",
      width: "w-24",
    },
  ];
}

function calcAge(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}

// Map indicator to a numeric rank so sorting puts green > orange > red > none.
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
