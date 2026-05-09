import {
  useLayoutEffect,
  useMemo,
  useRef,
  type ChangeEvent,
  type KeyboardEvent,
  type MouseEvent,
} from "react";
import { useTranslation } from "react-i18next";
import Table, { type Column } from "@/shared/components/Table";
import { CategoryChips } from "@/shared/components/CategoryChip";
import SocialLink from "@/shared/components/SocialLink";
import { calcAge } from "@/shared/utils/age";
import { formatDateTimeShort } from "@/shared/utils/formatDateTime";
import type { CampaignCreatorStatus } from "@/api/campaignCreators";
import { CAMPAIGN_CREATOR_STATUS } from "@/shared/constants/campaignCreatorStatus";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

export type SelectAllState = "unchecked" | "indeterminate" | "checked";

interface CampaignCreatorsTableProps {
  rows: CampaignCreatorRow[];
  status: CampaignCreatorStatus;
  selectedKey?: string;
  onRowClick: (row: CampaignCreatorRow) => void;
  onRemove?: (row: CampaignCreatorRow) => void;
  emptyMessage: string;
  checkedCreatorIds?: Set<string>;
  onToggleOne?: (creatorId: string) => void;
  onToggleAll?: () => void;
  selectAllState?: SelectAllState;
  selectAllTestId?: string;
  rowSelectionDisabled?: boolean;
}

export default function CampaignCreatorsTable({
  rows,
  status,
  selectedKey,
  onRowClick,
  onRemove,
  emptyMessage,
  checkedCreatorIds,
  onToggleOne,
  onToggleAll,
  selectAllState,
  selectAllTestId,
  rowSelectionDisabled,
}: CampaignCreatorsTableProps) {
  const { t } = useTranslation("creators");
  const { t: tCampaigns } = useTranslation("campaigns");

  const columns = useMemo<Column<CampaignCreatorRow>[]>(
    () =>
      buildColumns(t, tCampaigns, {
        status,
        onRemove,
        checkedCreatorIds,
        onToggleOne,
        onToggleAll,
        selectAllState: selectAllState ?? "unchecked",
        selectAllTestId,
        selectionDisabled: rowSelectionDisabled ?? false,
      }),
    [
      t,
      tCampaigns,
      status,
      onRemove,
      checkedCreatorIds,
      onToggleOne,
      onToggleAll,
      selectAllState,
      selectAllTestId,
      rowSelectionDisabled,
    ],
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

interface BuildColumnsOpts {
  status: CampaignCreatorStatus;
  onRemove?: (row: CampaignCreatorRow) => void;
  checkedCreatorIds?: Set<string>;
  onToggleOne?: (creatorId: string) => void;
  onToggleAll?: () => void;
  selectAllState: SelectAllState;
  selectAllTestId?: string;
  selectionDisabled: boolean;
}

function buildColumns(
  t: (key: string) => string,
  tCampaigns: (key: string, opts?: Record<string, unknown>) => string,
  opts: BuildColumnsOpts,
): Column<CampaignCreatorRow>[] {
  const placeholder = tCampaigns("campaignCreators.deletedPlaceholder");
  const deletedTitle = tCampaigns("campaignCreators.creatorDeleted");

  const checkbox: Column<CampaignCreatorRow>[] = opts.checkedCreatorIds
    ? [
        {
          key: "checkbox",
          width: "w-10",
          header: (
            <SelectAllCheckbox
              state={opts.selectAllState}
              onToggle={opts.onToggleAll}
              testid={opts.selectAllTestId ?? "campaign-creators-select-all"}
              ariaLabel={
                opts.selectAllState === "checked"
                  ? tCampaigns("campaignCreators.deselectAll")
                  : tCampaigns("campaignCreators.selectAll")
              }
              disabled={opts.selectionDisabled}
            />
          ),
          render: (row) => (
            <RowCheckbox
              creatorId={row.campaignCreator.creatorId}
              checked={
                opts.checkedCreatorIds?.has(row.campaignCreator.creatorId) ??
                false
              }
              onToggle={opts.onToggleOne}
              ariaLabel={tCampaigns("campaignCreators.selectAria", {
                name: row.creator
                  ? `${row.creator.lastName} ${row.creator.firstName}`
                  : tCampaigns("campaignCreators.creatorDeleted"),
              })}
              disabled={opts.selectionDisabled}
            />
          ),
        },
      ]
    : [];

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
  ];

  const extra = buildStatusColumns(opts.status, tCampaigns);

  const tail: Column<CampaignCreatorRow>[] = [
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

  const actions: Column<CampaignCreatorRow>[] = opts.onRemove
    ? [
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
                opts.onRemove?.(row);
              }}
              className="rounded-button p-1 text-gray-400 transition hover:bg-red-50 hover:text-red-600"
            >
              <TrashIcon />
            </button>
          ),
          align: "right",
        },
      ]
    : [];

  return [...checkbox, ...base, ...extra, ...tail, ...actions];
}

function buildStatusColumns(
  status: CampaignCreatorStatus,
  tCampaigns: (key: string) => string,
): Column<CampaignCreatorRow>[] {
  switch (status) {
    case CAMPAIGN_CREATOR_STATUS.PLANNED:
      return [];
    case CAMPAIGN_CREATOR_STATUS.INVITED:
      return [
        pairColumn(
          "invited",
          tCampaigns("campaignCreators.columns.invited"),
          (row) => row.campaignCreator.invitedCount,
          (row) => row.campaignCreator.invitedAt ?? null,
        ),
        pairColumn(
          "reminded",
          tCampaigns("campaignCreators.columns.reminded"),
          (row) => row.campaignCreator.remindedCount,
          (row) => row.campaignCreator.remindedAt ?? null,
        ),
      ];
    case CAMPAIGN_CREATOR_STATUS.DECLINED:
    case CAMPAIGN_CREATOR_STATUS.AGREED:
      return [
        pairColumn(
          "invited",
          tCampaigns("campaignCreators.columns.invited"),
          (row) => row.campaignCreator.invitedCount,
          (row) => row.campaignCreator.invitedAt ?? null,
        ),
        decidedColumn(
          tCampaigns("campaignCreators.columns.decided"),
          (row) => row.campaignCreator.decidedAt ?? null,
        ),
      ];
    default: {
      const _exhaustive: never = status;
      return _exhaustive;
    }
  }
}

function pairColumn(
  kind: "invited" | "reminded",
  header: string,
  getCount: (row: CampaignCreatorRow) => number,
  getIso: (row: CampaignCreatorRow) => string | null,
): Column<CampaignCreatorRow> {
  return {
    key: `${kind}-pair`,
    header,
    width: "w-40",
    render: (row) => (
      <span className="whitespace-nowrap text-gray-700">
        <span
          data-testid={`campaign-creator-${kind}-count-${row.campaignCreator.creatorId}`}
        >
          {getCount(row)}
        </span>
        <span className="mx-1 text-gray-400">·</span>
        <span
          data-testid={`campaign-creator-${kind}-at-${row.campaignCreator.creatorId}`}
        >
          {formatDateTimeShort(getIso(row))}
        </span>
      </span>
    ),
  };
}

function decidedColumn(
  header: string,
  getIso: (row: CampaignCreatorRow) => string | null,
): Column<CampaignCreatorRow> {
  return {
    key: "decided-at",
    header,
    width: "w-32",
    render: (row) => (
      <span
        className="text-gray-700"
        data-testid={`campaign-creator-decided-at-${row.campaignCreator.creatorId}`}
      >
        {formatDateTimeShort(getIso(row))}
      </span>
    ),
  };
}

interface SelectAllCheckboxProps {
  state: SelectAllState;
  onToggle?: () => void;
  testid: string;
  ariaLabel: string;
  disabled?: boolean;
}

function SelectAllCheckbox({
  state,
  onToggle,
  testid,
  ariaLabel,
  disabled,
}: SelectAllCheckboxProps) {
  const ref = useRef<HTMLInputElement>(null);
  // `indeterminate` is an HTML property, not a React attribute — apply it
  // synchronously before the browser paints to avoid a one-frame flicker.
  useLayoutEffect(() => {
    if (ref.current) {
      ref.current.indeterminate = state === "indeterminate";
    }
  }, [state]);

  function handleChange() {
    onToggle?.();
  }

  return (
    <input
      ref={ref}
      type="checkbox"
      checked={state === "checked"}
      onChange={handleChange}
      aria-label={ariaLabel}
      data-testid={testid}
      disabled={disabled}
      className="h-4 w-4 cursor-pointer rounded border-surface-300 disabled:cursor-not-allowed disabled:opacity-50"
    />
  );
}

interface RowCheckboxProps {
  creatorId: string;
  checked: boolean;
  onToggle?: (creatorId: string) => void;
  ariaLabel: string;
  disabled?: boolean;
}

function RowCheckbox({
  creatorId,
  checked,
  onToggle,
  ariaLabel,
  disabled,
}: RowCheckboxProps) {
  function handleChange(_e: ChangeEvent<HTMLInputElement>) {
    onToggle?.(creatorId);
  }

  function stopMouse(e: MouseEvent<HTMLDivElement>) {
    e.stopPropagation();
  }

  function stopKey(e: KeyboardEvent<HTMLDivElement>) {
    e.stopPropagation();
  }

  return (
    <div
      className="flex items-center"
      onClick={stopMouse}
      onKeyDown={stopKey}
      role="presentation"
    >
      <input
        type="checkbox"
        checked={checked}
        onChange={handleChange}
        aria-label={ariaLabel}
        data-testid={`campaign-creator-checkbox-${creatorId}`}
        disabled={disabled}
        className="h-4 w-4 cursor-pointer rounded border-surface-300 disabled:cursor-not-allowed disabled:opacity-50"
      />
    </div>
  );
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
