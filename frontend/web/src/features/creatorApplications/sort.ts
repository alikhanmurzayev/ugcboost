import type { components } from "@/api/generated/schema";

export type ApiSortField = components["schemas"]["CreatorApplicationListSortField"];
export type ApiOrder = components["schemas"]["SortOrder"];

export interface SortState {
  sort: ApiSortField;
  order: ApiOrder;
}

export const DEFAULT_SORT: SortState = { sort: "created_at", order: "desc" };

const VALID_SORT_FIELDS: readonly ApiSortField[] = [
  "created_at",
  "updated_at",
  "full_name",
  "birth_date",
  "city_name",
];

const VALID_ORDERS: readonly ApiOrder[] = ["asc", "desc"];

export function parseSortFromUrl(sp: URLSearchParams): SortState {
  const sortParam = sp.get("sort");
  const orderParam = sp.get("order");
  const sort = (VALID_SORT_FIELDS as readonly string[]).includes(sortParam ?? "")
    ? (sortParam as ApiSortField)
    : DEFAULT_SORT.sort;
  const order = (VALID_ORDERS as readonly string[]).includes(orderParam ?? "")
    ? (orderParam as ApiOrder)
    : DEFAULT_SORT.order;
  return { sort, order };
}

export function serializeSort(sp: URLSearchParams, sortState: SortState): void {
  if (
    sortState.sort === DEFAULT_SORT.sort &&
    sortState.order === DEFAULT_SORT.order
  ) {
    sp.delete("sort");
    sp.delete("order");
  } else {
    sp.set("sort", sortState.sort);
    sp.set("order", sortState.order);
  }
}

export function toggleSort(prev: SortState, field: ApiSortField): SortState {
  if (prev.sort !== field) return { sort: field, order: "desc" };
  return { sort: field, order: prev.order === "asc" ? "desc" : "asc" };
}

const COLUMN_TO_FIELD: Record<string, ApiSortField | undefined> = {
  fullName: "full_name",
  submittedAt: "created_at",
  hoursInStage: "created_at",
  city: "city_name",
  birthDate: "birth_date",
};

export function fieldForColumn(columnKey: string): ApiSortField | undefined {
  return COLUMN_TO_FIELD[columnKey];
}
