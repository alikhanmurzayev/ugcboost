import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { listCampaigns } from "@/_prototype/api/campaigns";
import { campaignKeys } from "@/_prototype/queryKeys";
import { ROUTES } from "@/_prototype/routes";
import Spinner from "@/_prototype/shared/components/Spinner";
import ErrorState from "@/_prototype/shared/components/ErrorState";
import ApplicationsTable, {
  type Column,
} from "@/_prototype/features/creatorApplications/components/ApplicationsTable";
import { sortApplications, type SortState } from "@/_prototype/features/creatorApplications/sort";
import CampaignStatusBadge from "./components/CampaignStatusBadge";
import type { Campaign, CampaignStatus } from "./types";

interface Props {
  status: CampaignStatus;
}

export default function CampaignsPage({ status }: Props) {
  const { t } = useTranslation("prototype_campaigns");
  const navigate = useNavigate();
  const [sort, setSort] = useState<SortState>({
    key: "createdAt",
    dir: "desc",
  });
  const [search, setSearch] = useState("");

  const listQuery = useQuery({
    queryKey: campaignKeys.list(status),
    queryFn: () => listCampaigns(status),
  });

  // Drafts and rejected campaigns are editable — clicking the title goes to the
  // editor; otherwise to the read-only detail view.
  const editable = status === "draft" || status === "rejected";
  const columns: Column<Campaign>[] = useMemo(() => buildColumns(t), [t]);

  function handleRowClick(row: Campaign) {
    navigate(
      "/" +
        (editable ? ROUTES.CAMPAIGN_EDIT(row.id) : ROUTES.CAMPAIGN_DETAIL(row.id)),
    );
  }

  const filtered = useMemo(() => {
    const rows = listQuery.data ?? [];
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((c) => c.title.toLowerCase().includes(q));
  }, [listQuery.data, search]);

  const sortedRows = useMemo(
    () => sortApplications(filtered, sort, columns),
    [filtered, sort, columns],
  );

  const totalCount = listQuery.data?.length ?? 0;

  return (
    <div data-testid={`campaigns-page-${status}`}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-3">
          <h1 className="text-2xl font-bold text-gray-900">
            {t(`headings.${status}`)}
          </h1>
          {!listQuery.isLoading && (
            <span
              className="text-lg font-medium text-gray-400"
              data-testid="campaigns-total"
            >
              {totalCount}
            </span>
          )}
        </div>
        <Link
          to={"/prototype/" + ROUTES.CAMPAIGN_NEW}
          className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600"
          data-testid="create-campaign-button"
        >
          {t("createCampaign")}
        </Link>
      </div>
      <p className="mt-1 text-sm text-gray-500">
        {t(`headingDescriptions.${status}`)}
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
          <div className="mt-4">
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("columns.title")}
              aria-label={t("columns.title")}
              className="w-72 rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
              data-testid="campaigns-search"
            />
          </div>

          <ApplicationsTable
            rows={sortedRows}
            columns={columns}
            rowKey={(row) => row.id}
            sort={sort}
            onSortChange={setSort}
            onRowClick={handleRowClick}
            emptyMessage={search ? t("emptyFiltered") : t("empty")}
          />
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
      key: "title",
      header: t("columns.title"),
      render: (row) => (
        <span
          className="font-medium text-gray-900"
          data-testid={`campaign-title-${row.id}`}
        >
          {row.title}
        </span>
      ),
      sortValue: (row) => row.title.toLowerCase(),
    },
    {
      key: "type",
      header: t("columns.type"),
      render: (row) => t(`types.${row.type}`),
      sortValue: (row) => row.type,
      width: "w-40",
    },
    {
      key: "status",
      header: t("columns.status"),
      render: (row) => <CampaignStatusBadge status={row.status} />,
      width: "w-36",
    },
    {
      key: "creators",
      header: t("columns.creators"),
      render: (row) => row.creatorsCount,
      sortValue: (row) => row.creatorsCount,
      align: "right",
      width: "w-20",
    },
    {
      key: "deadline",
      header: t("columns.deadline"),
      render: (row) => formatShortDate(row.publishDeadline),
      sortValue: (row) => row.publishDeadline,
      width: "w-28",
    },
    {
      key: "createdAt",
      header: t("columns.createdAt"),
      render: (row) => formatShortDate(row.createdAt),
      sortValue: (row) => row.createdAt,
      width: "w-28",
    },
  ];
}

function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
    year: "2-digit",
  });
}
