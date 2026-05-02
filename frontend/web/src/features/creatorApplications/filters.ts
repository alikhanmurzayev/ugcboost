import type { components } from "@/api/generated/schema";
import type { SortState } from "./sort";

export type ListInput = components["schemas"]["CreatorApplicationsListRequest"];
type Status = components["schemas"]["CreatorApplicationStatus"];

export interface FilterValues {
  search?: string;
  dateFrom?: string;
  dateTo?: string;
  cities: string[];
  ageFrom?: number;
  ageTo?: number;
  categories: string[];
}

const FILTER_PARAM_KEYS = [
  "q",
  "dateFrom",
  "dateTo",
  "cities",
  "ageFrom",
  "ageTo",
  "categories",
] as const;

export function parseFilters(sp: URLSearchParams): FilterValues {
  return {
    search: sp.get("q") ?? undefined,
    dateFrom: parseIsoDate(sp.get("dateFrom")),
    dateTo: parseIsoDate(sp.get("dateTo")),
    cities: splitCsv(sp.get("cities")),
    ageFrom: parsePositiveInt(sp.get("ageFrom")),
    ageTo: parsePositiveInt(sp.get("ageTo")),
    categories: splitCsv(sp.get("categories")),
  };
}

export function writeFilters(sp: URLSearchParams, f: FilterValues): void {
  setOrDelete(sp, "q", f.search);
  setOrDelete(sp, "dateFrom", f.dateFrom);
  setOrDelete(sp, "dateTo", f.dateTo);
  setOrDelete(sp, "cities", f.cities.length ? f.cities.join(",") : undefined);
  setOrDelete(
    sp,
    "ageFrom",
    f.ageFrom !== undefined ? String(f.ageFrom) : undefined,
  );
  setOrDelete(sp, "ageTo", f.ageTo !== undefined ? String(f.ageTo) : undefined);
  setOrDelete(
    sp,
    "categories",
    f.categories.length ? f.categories.join(",") : undefined,
  );
}

export function clearFilters(sp: URLSearchParams): void {
  for (const k of FILTER_PARAM_KEYS) sp.delete(k);
}

export function isFilterActive(f: FilterValues): boolean {
  return (
    !!f.search ||
    !!f.dateFrom ||
    !!f.dateTo ||
    f.cities.length > 0 ||
    f.ageFrom !== undefined ||
    f.ageTo !== undefined ||
    f.categories.length > 0
  );
}

export function countActive(f: FilterValues): number {
  let count = 0;
  if (f.dateFrom || f.dateTo) count++;
  if (f.cities.length > 0) count++;
  if (f.ageFrom !== undefined || f.ageTo !== undefined) count++;
  if (f.categories.length > 0) count++;
  return count;
}

interface ToListInputOptions {
  statuses: Status[];
  sort: SortState;
  page: number;
  perPage: number;
}

export function toListInput(
  filters: FilterValues,
  opts: ToListInputOptions,
): ListInput {
  const body: ListInput = {
    page: opts.page,
    perPage: opts.perPage,
    sort: opts.sort.sort,
    order: opts.sort.order,
    statuses: opts.statuses,
  };
  const search = filters.search?.trim();
  if (search) body.search = search;
  if (filters.dateFrom) body.dateFrom = `${filters.dateFrom}T00:00:00.000Z`;
  if (filters.dateTo) body.dateTo = `${filters.dateTo}T23:59:59.999Z`;
  if (filters.cities.length > 0) body.cities = filters.cities;
  if (filters.categories.length > 0) body.categories = filters.categories;
  if (filters.ageFrom !== undefined) body.ageFrom = filters.ageFrom;
  if (filters.ageTo !== undefined) body.ageTo = filters.ageTo;
  return body;
}

export function calcAge(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}

function splitCsv(value: string | null): string[] {
  if (!value) return [];
  return value.split(",").filter(Boolean);
}

function parsePositiveInt(value: string | null): number | undefined {
  if (!value) return undefined;
  const n = Number(value);
  return Number.isFinite(n) && n >= 0 ? Math.trunc(n) : undefined;
}

function parseIsoDate(value: string | null): string | undefined {
  if (!value) return undefined;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
  const d = new Date(`${value}T00:00:00Z`);
  return Number.isNaN(d.getTime()) ? undefined : value;
}

function setOrDelete(
  sp: URLSearchParams,
  key: string,
  value: string | undefined,
): void {
  if (value !== undefined && value !== "") sp.set(key, value);
  else sp.delete(key);
}
