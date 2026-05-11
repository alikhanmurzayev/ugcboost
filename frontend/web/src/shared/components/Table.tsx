import { type KeyboardEvent, type ReactNode } from "react";

export interface Column<T> {
  key: string;
  header: ReactNode;
  render: (row: T, index: number) => ReactNode;
  sortable?: boolean;
  align?: "left" | "right";
  width?: string;
}

interface TableProps<T> {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  sortColumn?: string;
  sortOrder?: "asc" | "desc";
  onSortChange?: (columnKey: string) => void;
  onRowClick?: (row: T) => void;
  selectedKey?: string;
  emptyMessage?: ReactNode;
  testid?: string;
}

export default function Table<T>({
  rows,
  columns,
  rowKey,
  sortColumn,
  sortOrder,
  onSortChange,
  onRowClick,
  selectedKey,
  emptyMessage,
  testid = "data-table",
}: TableProps<T>) {
  function handleHeaderClick(col: Column<T>) {
    if (!col.sortable || !onSortChange) return;
    onSortChange(col.key);
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
      <table className="w-full text-left text-sm" data-testid={testid}>
        <thead>
          <tr className="border-b border-surface-300 text-gray-500">
            {columns.map((col) => {
              const isActive = sortColumn === col.key;
              return (
                <th
                  key={col.key}
                  scope="col"
                  data-testid={`column-${col.key}`}
                  className={`whitespace-nowrap px-4 pb-2 first:pl-0 font-medium ${col.width ?? ""} ${
                    col.align === "right" ? "text-right" : "text-left"
                  }`}
                >
                  {col.sortable ? (
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
                        {isActive ? (sortOrder === "asc" ? "↑" : "↓") : "↕"}
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
            const interactive = !!onRowClick;
            return (
              <tr
                key={key}
                {...(interactive && {
                  role: "button",
                  tabIndex: 0,
                  onClick: () => onRowClick(row),
                  onKeyDown: (e) => handleRowKeyDown(e, row),
                })}
                className={`border-b border-surface-200 transition-colors ${
                  interactive ? "cursor-pointer" : ""
                } ${
                  isSelected
                    ? "bg-primary-100"
                    : interactive
                      ? "hover:bg-primary-50"
                      : ""
                }`}
                data-testid={`row-${key}`}
                data-selected={isSelected ? "true" : "false"}
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={`px-4 py-3 first:pl-0 ${col.align === "right" ? "text-right" : ""}`}
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
