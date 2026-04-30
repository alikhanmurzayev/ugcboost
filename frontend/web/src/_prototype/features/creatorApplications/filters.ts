import type { Application } from "./types";

export interface FilterValues {
  search?: string; // free-text query over name/IIN
  dateFrom?: string; // YYYY-MM-DD
  dateTo?: string; // YYYY-MM-DD
  cities: string[]; // city codes
  ageFrom?: number;
  ageTo?: number;
  categories: string[]; // category codes
}

const KEYS = [
  "search",
  "dateFrom",
  "dateTo",
  "cities",
  "ageFrom",
  "ageTo",
  "categories",
];

export function parseFilters(sp: URLSearchParams): FilterValues {
  return {
    search: sp.get("q") ?? undefined,
    dateFrom: sp.get("dateFrom") ?? undefined,
    dateTo: sp.get("dateTo") ?? undefined,
    cities: splitCsv(sp.get("cities")),
    ageFrom: parseIntOrUndefined(sp.get("ageFrom")),
    ageTo: parseIntOrUndefined(sp.get("ageTo")),
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
  sp.delete("q");
  for (const k of KEYS) sp.delete(k);
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

export function applyFilters(
  applications: Application[],
  f: FilterValues,
): Application[] {
  const query = f.search?.trim().toLowerCase().replace(/^@/, "");
  return applications.filter((app) => {
    if (query) {
      const haystack = [
        app.lastName,
        app.firstName,
        app.middleName ?? "",
        ...app.socials.map((s) => s.handle),
      ]
        .join(" ")
        .toLowerCase();
      if (!haystack.includes(query)) return false;
    }
    const submittedDay = app.createdAt.slice(0, 10);
    if (f.dateFrom && submittedDay < f.dateFrom) return false;
    if (f.dateTo && submittedDay > f.dateTo) return false;
    if (f.cities.length > 0 && !f.cities.includes(app.city.code)) return false;
    if (
      f.categories.length > 0 &&
      !app.categories.some((c) => f.categories.includes(c.code))
    ) {
      return false;
    }
    const age = calcAge(app.birthDate);
    if (f.ageFrom !== undefined && age < f.ageFrom) return false;
    if (f.ageTo !== undefined && age > f.ageTo) return false;
    return true;
  });
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

function parseIntOrUndefined(value: string | null): number | undefined {
  if (!value) return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

function setOrDelete(
  sp: URLSearchParams,
  key: string,
  value: string | undefined,
): void {
  if (value !== undefined && value !== "") sp.set(key, value);
  else sp.delete(key);
}
