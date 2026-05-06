import type { CampaignsListInput } from "@/api/campaigns";
import type { SortState } from "./sort";

export interface FilterValues {
  search?: string;
  showDeleted: boolean;
}

const FILTER_PARAM_KEYS = ["q", "showDeleted"] as const;

export function parseFilters(sp: URLSearchParams): FilterValues {
  return {
    search: sp.get("q") ?? undefined,
    showDeleted: sp.get("showDeleted") === "true",
  };
}

export function writeFilters(sp: URLSearchParams, f: FilterValues): void {
  setOrDelete(sp, "q", f.search?.trim() || undefined);
  setOrDelete(sp, "showDeleted", f.showDeleted ? "true" : undefined);
}

export function clearFilters(sp: URLSearchParams): void {
  for (const k of FILTER_PARAM_KEYS) sp.delete(k);
}

export function isFilterActive(f: FilterValues): boolean {
  return !!f.search || f.showDeleted;
}

interface ToListInputOptions {
  sort: SortState;
  page: number;
  perPage: number;
}

export function toListInput(
  filters: FilterValues,
  opts: ToListInputOptions,
): CampaignsListInput {
  const body: CampaignsListInput = {
    page: opts.page,
    perPage: opts.perPage,
    sort: opts.sort.sort,
    order: opts.sort.order,
  };
  const search = filters.search?.trim();
  if (search) body.search = search;
  if (!filters.showDeleted) body.isDeleted = false;
  return body;
}

function setOrDelete(
  sp: URLSearchParams,
  key: string,
  value: string | undefined,
): void {
  if (value !== undefined && value !== "") sp.set(key, value);
  else sp.delete(key);
}
