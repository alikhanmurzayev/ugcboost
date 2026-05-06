import type { components } from "@/api/generated/schema";

export type ApiSortField = components["schemas"]["CampaignListSortField"];
export type ApiOrder = components["schemas"]["SortOrder"];

export interface SortState {
  sort: ApiSortField;
  order: ApiOrder;
}

export const DEFAULT_SORT: SortState = { sort: "created_at", order: "desc" };

export type ColumnFieldMap = Record<string, ApiSortField | undefined>;

export const DEFAULT_COLUMN_TO_FIELD: ColumnFieldMap = {
  name: "name",
  createdAt: "created_at",
};

const VALID_SORT_FIELDS: readonly ApiSortField[] = [
  "created_at",
  "updated_at",
  "name",
];

const VALID_ORDERS: readonly ApiOrder[] = ["asc", "desc"];

function isApiSortField(value: string | null): value is ApiSortField {
  if (value === null) return false;
  for (const f of VALID_SORT_FIELDS) {
    if (f === value) return true;
  }
  return false;
}

function isApiOrder(value: string | null): value is ApiOrder {
  if (value === null) return false;
  for (const o of VALID_ORDERS) {
    if (o === value) return true;
  }
  return false;
}

export function parseSortFromUrl(
  sp: URLSearchParams,
  defaults: SortState = DEFAULT_SORT,
): SortState {
  const sortParam = sp.get("sort");
  const orderParam = sp.get("order");
  const sort = isApiSortField(sortParam) ? sortParam : defaults.sort;
  const order = isApiOrder(orderParam) ? orderParam : defaults.order;
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
