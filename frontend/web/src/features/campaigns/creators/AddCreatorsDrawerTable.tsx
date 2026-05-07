import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import Table, { type Column } from "@/shared/components/Table";
import { CategoryChips } from "@/shared/components/CategoryChip";
import SocialLink from "@/shared/components/SocialLink";
import { calcAge } from "@/shared/utils/age";
import type { CreatorListItem } from "@/api/creators";

interface AddCreatorsDrawerTableProps {
  rows: CreatorListItem[];
  selected: Set<string>;
  existingCreatorIds: Set<string>;
  capReached: boolean;
  onToggle: (id: string, isMember: boolean) => void;
  sortColumn?: string;
  sortOrder?: "asc" | "desc";
  onSortChange?: (columnKey: string) => void;
  emptyMessage: string;
}

export default function AddCreatorsDrawerTable({
  rows,
  selected,
  existingCreatorIds,
  capReached,
  onToggle,
  sortColumn,
  sortOrder,
  onSortChange,
  emptyMessage,
}: AddCreatorsDrawerTableProps) {
  const { t } = useTranslation("creators");
  const { t: tCampaigns } = useTranslation("campaigns");

  const columns = useMemo<Column<CreatorListItem>[]>(
    () =>
      buildColumns({
        t,
        tCampaigns,
        selected,
        existingCreatorIds,
        capReached,
        onToggle,
      }),
    [t, tCampaigns, selected, existingCreatorIds, capReached, onToggle],
  );

  return (
    <Table
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      sortColumn={sortColumn}
      sortOrder={sortOrder}
      onSortChange={onSortChange}
      emptyMessage={emptyMessage}
      testid="add-creators-drawer-table"
    />
  );
}

interface BuildColumnsOpts {
  t: (key: string) => string;
  tCampaigns: (key: string, options?: Record<string, unknown>) => string;
  selected: Set<string>;
  existingCreatorIds: Set<string>;
  capReached: boolean;
  onToggle: (id: string, isMember: boolean) => void;
}

function buildColumns({
  t,
  tCampaigns,
  selected,
  existingCreatorIds,
  capReached,
  onToggle,
}: BuildColumnsOpts): Column<CreatorListItem>[] {
  return [
    {
      key: "select",
      header: "",
      width: "w-10",
      render: (row) => {
        const isMember = existingCreatorIds.has(row.id);
        const isSelected = selected.has(row.id);
        const disabled = isMember || (capReached && !isSelected);
        const fullName = `${row.lastName} ${row.firstName}`;
        return (
          <input
            type="checkbox"
            checked={isSelected}
            disabled={disabled}
            onChange={() => onToggle(row.id, isMember)}
            aria-label={tCampaigns("campaignCreators.selectAria", {
              name: fullName,
            })}
            data-testid={`drawer-row-checkbox-${row.id}`}
            className="h-4 w-4 rounded border-gray-300 text-primary focus:ring-primary disabled:cursor-not-allowed disabled:opacity-50"
          />
        );
      },
    },
    {
      key: "fullName",
      header: t("columns.fullName"),
      render: (row) => {
        const isMember = existingCreatorIds.has(row.id);
        return (
          <div
            className={`flex items-center gap-2 font-medium ${
              isMember ? "text-gray-400" : "text-gray-900"
            }`}
          >
            <span>
              {row.lastName} {row.firstName}
            </span>
            {isMember && (
              <span
                className="rounded-full bg-surface-200 px-2 py-0.5 text-xs font-semibold text-gray-600"
                data-testid={`drawer-row-added-badge-${row.id}`}
              >
                {tCampaigns("campaignCreators.addedBadge")}
              </span>
            )}
          </div>
        );
      },
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
