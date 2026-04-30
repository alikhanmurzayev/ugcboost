import { type KeyboardEvent, type ReactNode } from "react";
import type { SortState } from "../sort";

export interface Column<T> {
  key: string;
  header: ReactNode;
  render: (row: T, index: number) => ReactNode;
  sortValue?: (row: T) => string | number;
  align?: "left" | "right";
  width?: string;
}

interface ApplicationsTableProps<T> {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  sort?: SortState;
  onSortChange?: (sort: SortState) => void;
  onRowClick?: (row: T) => void;
  selectedKey?: string;
  emptyMessage?: ReactNode;
  testid?: string;
  // When true, the table renders with `table-fixed` so column widths from
  // the `width` prop are strictly respected (long headers don't stretch the column).
  fixedLayout?: boolean;
  // When true, cells use tighter horizontal padding for dense tables.
  compact?: boolean;
}

export default function ApplicationsTable<T>({
  rows,
  columns,
  rowKey,
  sort,
  onSortChange,
  onRowClick,
  selectedKey,
  emptyMessage,
  testid = "applications-table",
  fixedLayout = false,
  compact = false,
}: ApplicationsTableProps<T>) {
  const cellPx = compact ? "px-2" : "px-4";
  function handleHeaderClick(col: Column<T>) {
    if (!col.sortValue || !onSortChange) return;
    const next: SortState =
      sort?.key === col.key
        ? { key: col.key, dir: sort.dir === "asc" ? "desc" : "asc" }
        : { key: col.key, dir: "desc" };
    onSortChange(next);
  }

  function handleRowKeyDown(e: KeyboardEvent<HTMLTableRowElement>, row: T) {
    if (!onRowClick) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onRowClick(row);
    }
  }

  if (rows.length === 0) {
    return (
      <p className="mt-6 text-gray-500" data-testid={`${testid}-empty`}>
        {emptyMessage ?? "Нет данных"}
      </p>
    );
  }

  return (
    <div className="mt-6 overflow-x-auto">
      <table
        className={`w-full text-left text-sm ${fixedLayout ? "table-fixed" : ""}`}
        data-testid={testid}
      >
        <thead>
        <tr className="border-b border-surface-300 text-gray-500">
          {columns.map((col) => {
            const sortable = !!col.sortValue && !!onSortChange;
            const isActive = sort?.key === col.key;
            return (
              <th
                key={col.key}
                scope="col"
                className={`whitespace-nowrap ${cellPx} pb-2 first:pl-0 font-medium ${col.width ?? ""} ${
                  col.align === "right" ? "text-right" : "text-left"
                }`}
              >
                {sortable ? (
                  <button
                    type="button"
                    onClick={() => handleHeaderClick(col)}
                    className="inline-flex items-center gap-1 hover:text-gray-900"
                    data-testid={`th-${col.key}`}
                  >
                    {col.header}
                    <span
                      className={`text-xs ${isActive ? "text-gray-900" : "text-gray-300"}`}
                      aria-hidden
                    >
                      {isActive ? (sort?.dir === "asc" ? "↑" : "↓") : "↕"}
                    </span>
                  </button>
                ) : (
                  col.header
                )}
              </th>
            );
          })}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, rowIndex) => {
          const key = rowKey(row);
          const isSelected = selectedKey === key;
          return (
            <tr
              key={key}
              {...(onRowClick && {
                role: "button",
                tabIndex: 0,
                onClick: () => onRowClick(row),
                onKeyDown: (e) => handleRowKeyDown(e, row),
              })}
              className={`border-b border-surface-200 transition-colors ${
                onRowClick ? "cursor-pointer" : ""
              } ${
                isSelected
                  ? "bg-primary-100"
                  : onRowClick
                    ? "hover:bg-primary-50"
                    : ""
              }`}
              data-testid={`row-${key}`}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={`${cellPx} py-3 first:pl-0 ${col.align === "right" ? "text-right" : ""}`}
                >
                  {col.render(row, rowIndex)}
                </td>
              ))}
            </tr>
          );
        })}
        </tbody>
      </table>
    </div>
  );
}
