import type { components } from "@/api/generated/schema";

export type ApiSortField = components["schemas"]["CreatorListSortField"];
export type ApiOrder = components["schemas"]["SortOrder"];

export interface SortState {
  sort: ApiSortField;
  order: ApiOrder;
}

export const DEFAULT_SORT: SortState = { sort: "full_name", order: "asc" };

export type ColumnFieldMap = Record<string, ApiSortField | undefined>;

export const DEFAULT_COLUMN_TO_FIELD: ColumnFieldMap = {
  fullName: "full_name",
  age: "birth_date",
  city: "city_name",
  createdAt: "created_at",
};

const VALID_SORT_FIELDS: readonly ApiSortField[] = [
  "created_at",
  "updated_at",
  "full_name",
  "birth_date",
  "city_name",
];

const VALID_ORDERS: readonly ApiOrder[] = ["asc", "desc"];

export function parseSortFromUrl(
  sp: URLSearchParams,
  defaults: SortState = DEFAULT_SORT,
): SortState {
  const sortParam = sp.get("sort");
  const orderParam = sp.get("order");
  const sort = (VALID_SORT_FIELDS as readonly string[]).includes(sortParam ?? "")
    ? (sortParam as ApiSortField)
    : defaults.sort;
  const order = (VALID_ORDERS as readonly string[]).includes(orderParam ?? "")
    ? (orderParam as ApiOrder)
    : defaults.order;
  return { sort, order };
}

export function serializeSort(
  sp: URLSearchParams,
  sortState: SortState,
  defaults: SortState = DEFAULT_SORT,
): void {
  if (sortState.sort === defaults.sort && sortState.order === defaults.order) {
    sp.delete("sort");
    sp.delete("order");
  } else {
    sp.set("sort", sortState.sort);
    sp.set("order", sortState.order);
  }
}

export function toggleSort(prev: SortState, field: ApiSortField): SortState {
  if (prev.sort !== field) return { sort: field, order: "asc" };
  return { sort: field, order: prev.order === "asc" ? "desc" : "asc" };
}

export function fieldForColumn(
  columnKey: string,
  map: ColumnFieldMap = DEFAULT_COLUMN_TO_FIELD,
): ApiSortField | undefined {
  return map[columnKey];
}

export function activeColumnForSort(
  field: ApiSortField,
  map: ColumnFieldMap = DEFAULT_COLUMN_TO_FIELD,
): string | undefined {
  for (const [column, mapped] of Object.entries(map)) {
    if (mapped === field) return column;
  }
  return undefined;
}
