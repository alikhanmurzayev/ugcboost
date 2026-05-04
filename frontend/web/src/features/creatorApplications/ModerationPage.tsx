import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getCreatorApplication,
  listCreatorApplications,
} from "@/api/creatorApplications";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import ApplicationsTable, { type Column } from "./components/ApplicationsTable";
import ApplicationActions from "./components/ApplicationActions";
import ApplicationDrawer from "./components/ApplicationDrawer";
import ApplicationFilters from "./components/ApplicationFilters";
import { CategoryChips } from "./components/CategoryChip";
import HoursBadge from "./components/HoursBadge";
import SocialLink from "./components/SocialLink";
import { hoursSince } from "./hours";
import { isFilterActive, parseFilters, toListInput } from "./filters";
import {
  DEFAULT_COLUMN_TO_FIELD,
  fieldForColumn,
  parseSortFromUrl,
  serializeSort,
  toggleSort,
  type ColumnFieldMap,
  type SortState,
} from "./sort";
import type { Application } from "./types";

const PER_PAGE = 50;
const STATUSES = ["moderation"] as const;
const DEFAULT_SORT: SortState = { sort: "updated_at", order: "asc" };
const COLUMN_TO_FIELD: ColumnFieldMap = {
  ...DEFAULT_COLUMN_TO_FIELD,
  hoursInStage: "updated_at",
};

export default function ModerationPage() {
  const { t } = useTranslation("creatorApplications");
  const { t: tCommon } = useTranslation("common");
  const [searchParams, setSearchParams] = useSearchParams();

  const filters = useMemo(() => parseFilters(searchParams), [searchParams]);
  const sortState = useMemo(
    () => parseSortFromUrl(searchParams, DEFAULT_SORT),
    [searchParams],
  );
  const page = parsePage(searchParams.get("page"));
  const selectedId = searchParams.get("id");

  const listInput = useMemo(
    () =>
      toListInput(filters, {
        statuses: [...STATUSES],
        sort: sortState,
        page,
        perPage: PER_PAGE,
      }),
    [filters, sortState, page],
  );

  const listQuery = useQuery({
    queryKey: creatorApplicationKeys.list(listInput),
    queryFn: () => listCreatorApplications(listInput),
  });

  const detailQuery = useQuery({
    queryKey: creatorApplicationKeys.detail(selectedId ?? ""),
    queryFn: () => getCreatorApplication(selectedId ?? ""),
    enabled: !!selectedId,
  });

  const items = listQuery.data?.data?.items ?? [];
  const total = listQuery.data?.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const idx = items.findIndex((r) => r.id === selectedId);
  const canPrev = idx > 0;
  const canNext = idx >= 0 && idx < items.length - 1;

  const columns: Column<Application>[] = useMemo(() => buildColumns(t), [t]);
  const activeColumn = activeColumnForSort(sortState.sort);

  function openApplication(id: string) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.set("id", id);
      return np;
    });
  }

  function closeApplication() {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.delete("id");
      return np;
    });
  }

  function goPrev() {
    const prevItem = items[idx - 1];
    if (prevItem) openApplication(prevItem.id);
  }

  function goNext() {
    const nextItem = items[idx + 1];
    if (nextItem) openApplication(nextItem.id);
  }

  function handleSortChange(columnKey: string) {
    const field = fieldForColumn(columnKey, COLUMN_TO_FIELD);
    if (!field) return;
    const next = toggleSort(sortState, field);
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      serializeSort(np, next, DEFAULT_SORT);
      np.delete("page");
      return np;
    });
  }

  function changePage(nextPage: number) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      if (nextPage <= 1) np.delete("page");
      else np.set("page", String(nextPage));
      return np;
    });
  }

  return (
    <div data-testid="creator-applications-moderation-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("stages.moderation.title")}
        {!listQuery.isLoading && !listQuery.isError && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="moderation-total"
          >
            {total}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">
        {t("stages.moderation.description")}
      </p>

      <ApplicationFilters showTelegramFilter={false} />

      {listQuery.isLoading ? (
        <Spinner className="mt-6" />
      ) : listQuery.isError ? (
        <ErrorState
          message={t("loadError")}
          onRetry={() => void listQuery.refetch()}
        />
      ) : (
        <>
          <ApplicationsTable
            rows={items}
            columns={columns}
            rowKey={(row) => row.id}
            sortColumn={activeColumn}
            sortOrder={sortState.order}
            onSortChange={handleSortChange}
            onRowClick={(row) => openApplication(row.id)}
            selectedKey={selectedId ?? undefined}
            emptyMessage={
              isFilterActive(filters) ? t("emptyFiltered") : t("empty")
            }
          />

          {totalPages > 1 && (
            <div
              className="mt-4 flex items-center justify-between"
              data-testid="pagination"
            >
              <button
                type="button"
                onClick={() => changePage(page - 1)}
                disabled={page <= 1}
                className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
                data-testid="pagination-prev"
              >
                {tCommon("prev")}
              </button>
              <span
                className="text-sm text-gray-500"
                data-testid="pagination-info"
              >
                {t("pagination.page", { page, total: totalPages })}
              </span>
              <button
                type="button"
                onClick={() => changePage(page + 1)}
                disabled={page >= totalPages}
                className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
                data-testid="pagination-next"
              >
                {tCommon("next")}
              </button>
            </div>
          )}
        </>
      )}

      <ApplicationDrawer
        application={detailQuery.data?.data}
        isLoading={detailQuery.isLoading}
        isError={detailQuery.isError}
        open={!!selectedId}
        onClose={closeApplication}
        onPrev={goPrev}
        onNext={goNext}
        canPrev={canPrev}
        canNext={canNext}
        footer={<ApplicationActions application={detailQuery.data?.data} />}
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
      header: t("columns.fullName"),
      render: (row) => (
        <span className="font-medium text-gray-900">
          {row.lastName} {row.firstName}
        </span>
      ),
      sortable: true,
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
      key: "city",
      header: t("columns.city"),
      render: (row) => (
        <span className="text-gray-700">{row.city.name}</span>
      ),
      sortable: true,
      width: "w-32",
    },
    {
      key: "submittedAt",
      header: t("columns.submittedAt"),
      render: (row) => formatShortDate(row.createdAt),
      sortable: true,
      width: "w-24",
    },
    {
      key: "hoursInStage",
      header: t("columns.hoursInStage"),
      render: (row) => <HoursBadge hours={hoursSince(row.updatedAt)} />,
      align: "right",
      width: "w-24",
      sortable: true,
    },
  ];
}

function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
  });
}

function activeColumnForSort(field: string): string | undefined {
  if (field === "full_name") return "fullName";
  if (field === "created_at") return "submittedAt";
  if (field === "updated_at") return "hoursInStage";
  if (field === "city_name") return "city";
  return undefined;
}

function parsePage(value: string | null): number {
  if (!value) return 1;
  const n = Number(value);
  return Number.isFinite(n) && n >= 1 ? Math.trunc(n) : 1;
}
