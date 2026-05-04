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
  fieldForColumn,
  parseSortFromUrl,
  serializeSort,
  toggleSort,
} from "./sort";
import type { Application } from "./types";

const PER_PAGE = 50;
const STATUSES = ["verification"] as const;

export default function VerificationPage() {
  const { t } = useTranslation("creatorApplications");
  const { t: tCommon } = useTranslation("common");
  const [searchParams, setSearchParams] = useSearchParams();

  const filters = useMemo(() => parseFilters(searchParams), [searchParams]);
  const sortState = useMemo(() => parseSortFromUrl(searchParams), [searchParams]);
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
    <div data-testid="creator-applications-verification-page">
      <h1 className="flex items-baseline gap-3 text-2xl font-bold text-gray-900">
        {t("stages.verification.title")}
        {!listQuery.isLoading && !listQuery.isError && (
          <span
            className="text-lg font-medium text-gray-400"
            data-testid="verification-total"
          >
            {total}
          </span>
        )}
      </h1>
      <p className="mt-1 text-sm text-gray-500">
        {t("stages.verification.description")}
      </p>

      <ApplicationFilters />

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
      key: "telegram",
      header: t("columns.telegram"),
      render: (row) => (
        <TelegramCell
          linked={row.telegramLinked}
          linkedLabel={t("telegramLinked")}
          notLinkedLabel={t("telegramNotLinked")}
        />
      ),
      align: "right",
      width: "w-20",
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
      render: (row) => <HoursBadge hours={hoursSince(row.createdAt)} />,
      align: "right",
      width: "w-24",
    },
  ];
}

function TelegramCell({
  linked,
  linkedLabel,
  notLinkedLabel,
}: {
  linked: boolean;
  linkedLabel: string;
  notLinkedLabel: string;
}) {
  const label = linked ? linkedLabel : notLinkedLabel;
  return (
    <span
      title={label}
      aria-label={label}
      data-testid={
        linked ? "row-telegram-linked" : "row-telegram-not-linked"
      }
      className={`inline-flex h-5 w-5 items-center justify-center ${
        linked ? "text-sky-500" : "text-gray-300"
      }`}
    >
      <TelegramIcon />
    </span>
  );
}

function TelegramIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M9.78 18.65l.28-4.23 7.68-6.92c.34-.31-.07-.46-.52-.19L7.74 13.3 3.64 12c-.88-.25-.89-.86.2-1.3L19.52 4.6c.73-.33 1.43.18 1.15 1.3l-3.05 13.53c-.19.85-.7 1.06-1.42.66l-3.92-2.9-1.88 1.83c-.22.22-.4.4-.82.4l.3-2.27z" />
    </svg>
  );
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
  return undefined;
}

function parsePage(value: string | null): number {
  if (!value) return 1;
  const n = Number(value);
  return Number.isFinite(n) && n >= 1 ? Math.trunc(n) : 1;
}
