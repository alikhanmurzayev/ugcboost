import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { listDictionary } from "@/api/dictionaries";
import { dictionaryKeys } from "@/shared/constants/queryKeys";
import {
  clearFilters,
  countActive,
  isFilterActive,
  parseFilters,
  writeFilters,
  type FilterValues,
} from "../filters";
import DateRangePicker from "./DateRangePicker";
import SearchableMultiselect from "./SearchableMultiselect";

interface ApplicationFiltersProps {
  showTelegramFilter?: boolean;
}

export default function ApplicationFilters({
  showTelegramFilter = true,
}: ApplicationFiltersProps = {}) {
  const { t } = useTranslation("creatorApplications");
  const [searchParams, setSearchParams] = useSearchParams();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const filters = parseFilters(searchParams);
  const activeCount = countActive(filters);
  const active = isFilterActive(filters);

  const citiesQuery = useQuery({
    queryKey: dictionaryKeys.list("cities"),
    queryFn: () => listDictionary("cities"),
    staleTime: 5 * 60 * 1000,
    enabled: open,
  });
  const categoriesQuery = useQuery({
    queryKey: dictionaryKeys.list("categories"),
    queryFn: () => listDictionary("categories"),
    staleTime: 5 * 60 * 1000,
    enabled: open,
  });

  useEffect(() => {
    if (!open) return;

    function handlePointer(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }

    window.addEventListener("mousedown", handlePointer);
    window.addEventListener("keydown", handleKey);
    return () => {
      window.removeEventListener("mousedown", handlePointer);
      window.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  function update(next: FilterValues) {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      writeFilters(np, next);
      np.delete("page");
      return np;
    });
  }

  function reset() {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      clearFilters(np);
      np.delete("page");
      return np;
    });
  }

  return (
    <div
      ref={containerRef}
      className="relative mt-4 flex flex-wrap items-center gap-2"
      data-testid="application-filters"
    >
      <div className="relative">
        <SearchIcon />
        <input
          type="search"
          value={filters.search ?? ""}
          onChange={(e) =>
            update({ ...filters, search: e.target.value || undefined })
          }
          placeholder={t("filters.search")}
          aria-label={t("filters.search")}
          className="w-72 rounded-button border border-surface-300 bg-white py-1.5 pl-9 pr-3 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
          data-testid="filters-search"
        />
      </div>
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        aria-haspopup="dialog"
        data-testid="filters-toggle"
        className="inline-flex items-center gap-2 rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100"
      >
        <FilterIcon />
        {t("filters.open")}
        {activeCount > 0 && (
          <span className="inline-flex min-w-[1.25rem] items-center justify-center rounded-full bg-primary px-1.5 py-0.5 text-xs font-semibold text-white">
            {activeCount}
          </span>
        )}
      </button>
      {active && (
        <button
          type="button"
          onClick={reset}
          className="text-sm font-medium text-gray-500 hover:text-gray-900"
          data-testid="filters-reset"
        >
          {t("filters.reset")}
        </button>
      )}

      {open && (
        <div
          role="dialog"
          aria-label={t("filters.title")}
          data-testid="filters-popover"
          className="absolute left-0 top-full z-30 mt-2 w-[480px] max-w-[calc(100vw-2rem)] rounded-card border border-surface-300 bg-white p-4 shadow-lg"
        >
          <FilterRow label={t("filters.date")}>
            <DateRangePicker
              from={filters.dateFrom}
              to={filters.dateTo}
              onChange={(from, to) =>
                update({ ...filters, dateFrom: from, dateTo: to })
              }
              testid="filter-date-range"
            />
          </FilterRow>

          <FilterRow label={t("filters.age")} className="mt-3">
            <div className="flex items-center gap-2">
              <input
                type="number"
                min={14}
                max={100}
                value={filters.ageFrom ?? ""}
                onChange={(e) =>
                  update({
                    ...filters,
                    ageFrom: e.target.value
                      ? Number(e.target.value)
                      : undefined,
                  })
                }
                className="w-20 rounded-button border border-gray-300 px-2 py-1 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-100"
                data-testid="filter-age-from"
                aria-label={t("filters.ageFrom")}
              />
              <span className="text-gray-400">—</span>
              <input
                type="number"
                min={14}
                max={100}
                value={filters.ageTo ?? ""}
                onChange={(e) =>
                  update({
                    ...filters,
                    ageTo: e.target.value ? Number(e.target.value) : undefined,
                  })
                }
                className="w-20 rounded-button border border-gray-300 px-2 py-1 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-100"
                data-testid="filter-age-to"
                aria-label={t("filters.ageTo")}
              />
            </div>
          </FilterRow>

          <FilterRow label={t("filters.city")} className="mt-3">
            <SearchableMultiselect
              options={citiesQuery.data?.data?.items ?? []}
              selected={filters.cities}
              onChange={(cities) => update({ ...filters, cities })}
              placeholder={t("filters.anyCity")}
              searchPlaceholder={t("filters.searchCity")}
              isLoading={citiesQuery.isLoading}
              testid="filter-cities"
            />
          </FilterRow>

          <FilterRow label={t("filters.categories")} className="mt-3">
            <SearchableMultiselect
              options={categoriesQuery.data?.data?.items ?? []}
              selected={filters.categories}
              onChange={(categories) => update({ ...filters, categories })}
              placeholder={t("filters.anyCategory")}
              searchPlaceholder={t("filters.searchCategory")}
              isLoading={categoriesQuery.isLoading}
              testid="filter-categories"
            />
          </FilterRow>

          {showTelegramFilter && (
            <FilterRow label={t("filters.telegram")} className="mt-3">
              <TelegramLinkedSegment
                value={filters.telegramLinked}
                onChange={(telegramLinked) =>
                  update({ ...filters, telegramLinked })
                }
                labels={{
                  any: t("filters.anyTelegram"),
                  linked: t("filters.telegramLinked"),
                  notLinked: t("filters.telegramNotLinked"),
                }}
              />
            </FilterRow>
          )}
        </div>
      )}
    </div>
  );
}

interface TelegramLinkedSegmentProps {
  value: boolean | undefined;
  onChange: (value: boolean | undefined) => void;
  labels: { any: string; linked: string; notLinked: string };
}

function TelegramLinkedSegment({
  value,
  onChange,
  labels,
}: TelegramLinkedSegmentProps) {
  const options: { key: string; label: string; value: boolean | undefined }[] =
    [
      { key: "any", label: labels.any, value: undefined },
      { key: "true", label: labels.linked, value: true },
      { key: "false", label: labels.notLinked, value: false },
    ];
  return (
    <div
      role="radiogroup"
      aria-label={labels.any}
      className="inline-flex rounded-button border border-surface-300 bg-white p-0.5"
      data-testid="filter-telegram-linked"
    >
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.key}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(opt.value)}
            data-testid={`filter-telegram-linked-${opt.key}`}
            className={`rounded-button px-3 py-1 text-sm font-medium transition ${
              active
                ? "bg-primary text-white"
                : "text-gray-700 hover:bg-surface-100"
            }`}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

function FilterRow({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={className}>
      <span className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </span>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}

function FilterIcon() {
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
      <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
    </svg>
  );
}

function SearchIcon() {
  return (
    <svg
      className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}
