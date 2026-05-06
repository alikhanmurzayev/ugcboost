import { useLayoutEffect, useMemo, useRef } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { listCreators, getCreator } from "@/api/creators";
import { creatorKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import Table, { type Column } from "@/shared/components/Table";
import { CategoryChips } from "@/shared/components/CategoryChip";
import SocialLink from "@/shared/components/SocialLink";
import { calcAge } from "@/shared/utils/age";
import { readCurrentId } from "@/shared/utils/readCurrentId";
import CreatorFilters from "./CreatorFilters";
import CreatorDrawer from "./CreatorDrawer";
import { isFilterActive, parseFilters, toListInput } from "./filters";
import {
  activeColumnForSort,
  fieldForColumn,
  parseSortFromUrl,
  serializeSort,
  toggleSort,
} from "./sort";
import type { CreatorListItem } from "./types";

const PER_PAGE = 50;

export default function CreatorsListPage() {
  const { t } = useTranslation("creators");
  const { t: tCommon } = useTranslation("common");
  const [searchParams, setSearchParams] = useSearchParams();

  const filters = useMemo(() => parseFilters(searchParams), [searchParams]);
  const sortState = useMemo(() => parseSortFromUrl(searchParams), [searchParams]);
  const page = parsePage(searchParams.get("page"));
  const selectedId = searchParams.get("id");

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
    queryKey: creatorKeys.list(listInput),
    queryFn: () => listCreators(listInput),
  });

  const detailQuery = useQuery({
    queryKey: creatorKeys.detail(selectedId ?? ""),
    queryFn: () => getCreator(selectedId ?? ""),
    enabled: !!selectedId,
  });

  const items = listQuery.data?.data?.items ?? [];
  const total = listQuery.data?.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const idx = items.findIndex((r) => r.id === selectedId);
  const canPrev = idx > 0;
  const canNext = idx >= 0 && idx < items.length - 1;
  const prefill = idx >= 0 ? items[idx] : undefined;

  // Rationale for the URL+ref read pattern: see shared/utils/readCurrentId.
  const selectedIdRef = useRef<string | null>(selectedId);
  const itemsRef = useRef<CreatorListItem[]>(items);
  useLayoutEffect(() => {
    selectedIdRef.current = selectedId;
    itemsRef.current = items;
  });

  const columns: Column<CreatorListItem>[] = useMemo(() => buildColumns(t), [t]);
  const activeColumn = activeColumnForSort(sortState.sort);

  function openCreator(id: string) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.set("id", id);
      return np;
    });
  }

  function closeCreator() {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.delete("id");
      return np;
    });
  }

  function goPrev() {
    const currentId = readCurrentId(selectedIdRef);
    if (!currentId) return;
    const list = itemsRef.current;
    const i = list.findIndex((r) => r.id === currentId);
    const prev = i > 0 ? list[i - 1] : undefined;
    if (prev) openCreator(prev.id);
  }

  function goNext() {
    const currentId = readCurrentId(selectedIdRef);
    if (!currentId) return;
    const list = itemsRef.current;
    const i = list.findIndex((r) => r.id === currentId);
    const next = i >= 0 ? list[i + 1] : undefined;
    if (next) openCreator(next.id);
  }

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

  function changePage(nextPage: number) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      if (nextPage <= 1) np.delete("page");
      else np.set("page", String(nextPage));
      return np;
    });
  }

  return (
    <div data-testid="creators-list-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("title")}
        {!listQuery.isLoading && !listQuery.isError && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="creators-total"
          >
            {total}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">{t("description")}</p>

      <CreatorFilters />

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
            onRowClick={(row) => openCreator(row.id)}
            selectedKey={selectedId ?? undefined}
            emptyMessage={
              isFilterActive(filters) ? t("emptyFiltered") : t("empty")
            }
            testid="creators-table"
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

      <CreatorDrawer
        prefill={prefill}
        detail={detailQuery.data?.data}
        isLoading={detailQuery.isLoading}
        isError={detailQuery.isError}
        open={!!selectedId}
        onClose={closeCreator}
        onPrev={goPrev}
        onNext={goNext}
        canPrev={canPrev}
        canNext={canNext}
      />
    </div>
  );
}

function buildColumns(t: (key: string) => string): Column<CreatorListItem>[] {
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
      key: "age",
      header: t("columns.age"),
      render: (row) => calcAge(row.birthDate),
      sortable: true,
      align: "right",
      width: "w-20",
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
      key: "createdAt",
      header: t("columns.createdAt"),
      render: (row) => formatShortDate(row.createdAt),
      sortable: true,
      width: "w-24",
    },
  ];
}

function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
  });
}

const MAX_PAGE = 1_000_000;

function parsePage(value: string | null): number {
  if (!value) return 1;
  const n = Number(value);
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.min(MAX_PAGE, Math.trunc(n));
}
