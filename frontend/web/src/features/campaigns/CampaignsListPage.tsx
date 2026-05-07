import { useMemo } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { listCampaigns } from "@/api/campaigns";
import { campaignKeys } from "@/shared/constants/queryKeys";
import { ROUTES } from "@/shared/constants/routes";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import Table, { type Column } from "@/shared/components/Table";
import { isFilterActive, parseFilters, toListInput, writeFilters } from "./filters";
import {
  activeColumnForSort,
  fieldForColumn,
  parseSortFromUrl,
  serializeSort,
  toggleSort,
} from "./sort";
import type { Campaign } from "./types";

const PER_PAGE = 50;
const MAX_PAGE = 1_000_000;

export default function CampaignsListPage() {
  const { t } = useTranslation("campaigns");
  const { t: tCommon } = useTranslation("common");
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();

  const filters = useMemo(() => parseFilters(searchParams), [searchParams]);
  const sortState = useMemo(() => parseSortFromUrl(searchParams), [searchParams]);
  const page = parsePage(searchParams.get("page"));

  const listInput = useMemo(
    () =>
      toListInput(filters, {
        sort: sortState,
        page,
        perPage: PER_PAGE,
      }),
    [filters, sortState, page],
  );

  const listQuery = useQuery({
    queryKey: campaignKeys.list(listInput),
    queryFn: () => listCampaigns(listInput),
  });

  const items = listQuery.data?.data?.items ?? [];
  const total = listQuery.data?.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const columns: Column<Campaign>[] = useMemo(() => buildColumns(t), [t]);
  const activeColumn = activeColumnForSort(sortState.sort);

  function handleSortChange(columnKey: string) {
    const field = fieldForColumn(columnKey);
    if (!field) return;
    const next = toggleSort(sortState, field);
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      serializeSort(np, next);
      np.delete("page");
      return np;
    });
  }

  function handleSearchChange(value: string) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      writeFilters(np, { ...filters, search: value || undefined });
      np.delete("page");
      return np;
    });
  }

  function handleShowDeletedChange(checked: boolean) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      writeFilters(np, { ...filters, showDeleted: checked });
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
    <div data-testid="campaigns-list-page">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
            {t("title")}
            {!listQuery.isLoading && !listQuery.isError && (
              <span
                className="text-lg font-medium text-gray-400"
                data-testid="campaigns-total"
              >
                {total}
              </span>
            )}
          </h1>
          <p className="mt-1 text-sm text-gray-500">{t("description")}</p>
        </div>
        <Link
          to={`/${ROUTES.CAMPAIGN_NEW}`}
          className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white transition hover:bg-primary-600"
          data-testid="campaigns-create-button"
        >
          {t("createButton")}
        </Link>
      </div>

      <div
        className="mt-4 flex flex-wrap items-center gap-4"
        data-testid="campaigns-toolbar"
      >
        <div className="relative">
          <SearchIcon />
          <input
            type="search"
            value={filters.search ?? ""}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder={t("filters.search")}
            aria-label={t("filters.search")}
            maxLength={128}
            className="w-72 rounded-button border border-surface-300 bg-white py-1.5 pl-9 pr-3 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
            data-testid="campaigns-search"
          />
        </div>
        <label className="inline-flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={filters.showDeleted}
            onChange={(e) => handleShowDeletedChange(e.target.checked)}
            className="h-4 w-4 rounded border-surface-300 text-primary focus:ring-primary-100"
            data-testid="campaigns-show-deleted"
          />
          {t("filters.showDeleted")}
        </label>
      </div>

      {listQuery.isLoading ? (
        <Spinner className="mt-6" />
      ) : listQuery.isError ? (
        <ErrorState
          message={t("loadError")}
          onRetry={() => void listQuery.refetch()}
        />
      ) : (
        <>
          <Table
            rows={items}
            columns={columns}
            rowKey={(row) => row.id}
            sortColumn={activeColumn}
            sortOrder={sortState.order}
            onSortChange={handleSortChange}
            onRowClick={(row) => navigate(`/${ROUTES.CAMPAIGN_DETAIL(row.id)}`)}
            emptyMessage={
              isFilterActive(filters) ? t("emptyFiltered") : t("empty")
            }
            testid="campaigns-table"
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
    </div>
  );
}

function buildColumns(t: (key: string) => string): Column<Campaign>[] {
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
      key: "name",
      header: t("columns.name"),
      render: (row) => (
        <div className="flex items-center gap-2">
          <span
            className={`font-medium ${row.isDeleted ? "text-gray-400" : "text-gray-900"}`}
          >
            {row.name}
          </span>
          {row.isDeleted && (
            <span
              className="inline-flex items-center rounded-full bg-surface-200 px-2 py-0.5 text-xs font-medium text-gray-500"
              data-testid={`campaign-deleted-${row.id}`}
            >
              {t("labels.deletedBadge")}
            </span>
          )}
        </div>
      ),
      sortable: true,
    },
    {
      key: "tmaUrl",
      header: t("columns.tmaUrl"),
      render: (row) => (
        <span className="block max-w-xs truncate text-gray-700">{row.tmaUrl}</span>
      ),
    },
    {
      key: "createdAt",
      header: t("columns.createdAt"),
      render: (row) => (
        <span className="whitespace-nowrap">{formatShortDate(row.createdAt)}</span>
      ),
      sortable: true,
      width: "w-36",
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          type="button"
          disabled
          title={t("actions.deleteDisabledHint")}
          aria-label={t("actions.delete")}
          data-testid={`campaign-delete-${row.id}`}
          className="rounded-button border border-surface-300 px-3 py-1 text-sm text-gray-400 disabled:cursor-not-allowed"
        >
          {t("actions.delete")}
        </button>
      ),
      align: "right",
      width: "w-32",
    },
  ];
}

function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

function parsePage(value: string | null): number {
  if (!value) return 1;
  const n = Number(value);
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.min(MAX_PAGE, Math.trunc(n));
}

function SearchIcon() {
  return (
    <svg
      className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}
