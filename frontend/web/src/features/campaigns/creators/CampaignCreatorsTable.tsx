import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import Table, { type Column } from "@/shared/components/Table";
import { CategoryChips } from "@/shared/components/CategoryChip";
import SocialLink from "@/shared/components/SocialLink";
import { calcAge } from "@/shared/utils/age";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

interface CampaignCreatorsTableProps {
  rows: CampaignCreatorRow[];
  selectedKey?: string;
  onRowClick: (row: CampaignCreatorRow) => void;
  onRemove?: (row: CampaignCreatorRow) => void;
  emptyMessage: string;
}

export default function CampaignCreatorsTable({
  rows,
  selectedKey,
  onRowClick,
  onRemove,
  emptyMessage,
}: CampaignCreatorsTableProps) {
  const { t } = useTranslation("creators");
  const { t: tCampaigns } = useTranslation("campaigns");

  const columns = useMemo<Column<CampaignCreatorRow>[]>(
    () => buildColumns(t, tCampaigns, onRemove),
    [t, tCampaigns, onRemove],
  );

  return (
    <Table
      rows={rows}
      columns={columns}
      rowKey={(row) => row.campaignCreator.creatorId}
      onRowClick={onRowClick}
      selectedKey={selectedKey}
      emptyMessage={emptyMessage}
      testid="campaign-creators-table"
    />
  );
}

function buildColumns(
  t: (key: string) => string,
  tCampaigns: (key: string) => string,
  onRemove?: (row: CampaignCreatorRow) => void,
): Column<CampaignCreatorRow>[] {
  const placeholder = tCampaigns("campaignCreators.deletedPlaceholder");
  const deletedTitle = tCampaigns("campaignCreators.creatorDeleted");

  const base: Column<CampaignCreatorRow>[] = [
    {
      key: "index",
      header: t("columns.index"),
      render: (_row, index) => (
        <span className="text-gray-400">{index + 1}</span>
      ),
      width: "w-10",
    },
    {
      key: "fullName",
      header: t("columns.fullName"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span
              className="font-medium text-gray-400"
              title={deletedTitle}
              data-testid={`campaign-creator-deleted-${row.campaignCreator.creatorId}`}
            >
              {placeholder}
            </span>
          );
        }
        return (
          <span className="font-medium text-gray-900">
            {row.creator.lastName} {row.creator.firstName}
          </span>
        );
      },
    },
    {
      key: "socials",
      header: t("columns.socials"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span className="text-gray-400" title={deletedTitle}>
              {placeholder}
            </span>
          );
        }
        return (
          <div
            className="flex flex-col gap-1"
            onClick={(e) => e.stopPropagation()}
            role="presentation"
          >
            {row.creator.socials.map((s) => (
              <SocialLink
                key={`${s.platform}-${s.handle}`}
                platform={s.platform}
                handle={s.handle}
                showHandle
              />
            ))}
          </div>
        );
      },
      width: "w-44",
    },
    {
      key: "categories",
      header: t("columns.categories"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span className="text-gray-400" title={deletedTitle}>
              {placeholder}
            </span>
          );
        }
        return <CategoryChips categories={row.creator.categories} />;
      },
    },
    {
      key: "age",
      header: t("columns.age"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span className="text-gray-400" title={deletedTitle}>
              {placeholder}
            </span>
          );
        }
        return calcAge(row.creator.birthDate);
      },
      align: "right",
      width: "w-20",
    },
    {
      key: "city",
      header: t("columns.city"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span className="text-gray-400" title={deletedTitle}>
              {placeholder}
            </span>
          );
        }
        return <span className="text-gray-700">{row.creator.city.name}</span>;
      },
      width: "w-32",
    },
    {
      key: "createdAt",
      header: t("columns.createdAt"),
      render: (row) => {
        if (!row.creator) {
          return (
            <span className="text-gray-400" title={deletedTitle}>
              {placeholder}
            </span>
          );
        }
        return formatShortDate(row.creator.createdAt);
      },
      width: "w-24",
    },
  ];

  if (!onRemove) return base;

  return [
    ...base,
    {
      key: "actions",
      header: "",
      width: "w-10",
      render: (row) => (
        <button
          type="button"
          aria-label={tCampaigns("campaignCreators.removeAria")}
          data-testid={`campaign-creator-remove-${row.campaignCreator.creatorId}`}
          onClick={(e) => {
            e.stopPropagation();
            onRemove(row);
          }}
          className="rounded-button p-1 text-gray-400 transition hover:bg-red-50 hover:text-red-600"
        >
          <TrashIcon />
        </button>
      ),
      align: "right",
    },
  ];
}

function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
  });
}

function TrashIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6" />
      <path d="M14 11v6" />
      <path d="M9 6V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2" />
    </svg>
  );
}
