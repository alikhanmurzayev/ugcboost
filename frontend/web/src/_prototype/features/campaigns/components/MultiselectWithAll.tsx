import { useEffect, useMemo, useRef, useState } from "react";

interface Option {
  code: string;
  name: string;
}

interface Props {
  options: Option[];
  selected: string[];
  any: boolean;
  onChange: (next: { any: boolean; selected: string[] }) => void;
  placeholder: string;
  searchPlaceholder: string;
  isLoading?: boolean;
  allLabel: string;
  testid?: string;
}

export default function MultiselectWithAll({
  options,
  selected,
  any: isAny,
  onChange,
  placeholder,
  searchPlaceholder,
  isLoading,
  allLabel,
  testid = "category-select",
}: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handlePointer(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
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

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.name.toLowerCase().includes(q));
  }, [options, query]);

  function toggleAll() {
    if (isAny) onChange({ any: false, selected: [] });
    else onChange({ any: true, selected: [] });
  }

  function toggleOne(code: string) {
    if (isAny) {
      // "Все" was selected — switching off this one means "all except this"
      onChange({
        any: false,
        selected: options.filter((o) => o.code !== code).map((o) => o.code),
      });
      return;
    }
    if (selected.includes(code)) {
      onChange({
        any: false,
        selected: selected.filter((c) => c !== code),
      });
    } else {
      const next = [...selected, code];
      // If user manually selected every category, treat it as "any".
      if (next.length === options.length && options.length > 0) {
        onChange({ any: true, selected: [] });
      } else {
        onChange({ any: false, selected: next });
      }
    }
  }

  function clear() {
    onChange({ any: false, selected: [] });
  }

  const selectedCount = isAny ? options.length : selected.length;
  const display: string =
    isAny
      ? allLabel
      : selectedCount === 0
        ? placeholder
        : selectedCount <= 2
          ? options
              .filter((o) => selected.includes(o.code))
              .map((o) => o.name)
              .join(", ")
          : `${
              options.find((o) => o.code === selected[0])?.name ?? ""
            } +${selectedCount - 1}`;

  return (
    <div ref={ref} className="relative inline-block w-full">
      <button
        type="button"
        onClick={() => setOpen((p) => !p)}
        aria-haspopup="listbox"
        aria-expanded={open}
        data-testid={testid}
        className="inline-flex w-full items-center justify-between gap-2 rounded-button border border-gray-300 bg-white px-3 py-2 text-left text-sm transition hover:border-primary/40 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary-100"
      >
        <span className={selectedCount === 0 ? "text-gray-500" : "text-gray-900"}>
          {display}
        </span>
        <ChevronDown />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 w-full min-w-72 rounded-card border border-surface-300 bg-white shadow-lg">
          <div className="border-b border-surface-200 p-2">
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={searchPlaceholder}
              className="w-full rounded-button border border-gray-300 px-2 py-1 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-100"
              data-testid={`${testid}-search`}
              autoFocus
            />
          </div>
          <ul className="max-h-60 overflow-y-auto py-1" role="listbox">
            <li>
              <label className="flex cursor-pointer items-center gap-2 border-b border-surface-200 bg-surface-50 px-3 py-2 text-sm font-medium hover:bg-primary-50">
                <input
                  type="checkbox"
                  checked={isAny}
                  onChange={toggleAll}
                  className="h-4 w-4 rounded border-gray-300 accent-primary text-primary focus:ring-primary"
                  data-testid={`${testid}-all`}
                />
                <span className="text-gray-900">{allLabel}</span>
              </label>
            </li>
            {isLoading && (
              <li className="px-3 py-2 text-sm text-gray-400">Загрузка...</li>
            )}
            {!isLoading && filtered.length === 0 && (
              <li className="px-3 py-2 text-sm text-gray-400">
                Ничего не найдено
              </li>
            )}
            {filtered.map((o) => {
              const isChecked = isAny || selected.includes(o.code);
              return (
                <li key={o.code}>
                  <label className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm hover:bg-primary-50">
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onChange={() => toggleOne(o.code)}
                      className="h-4 w-4 rounded border-gray-300 accent-primary text-primary focus:ring-primary"
                      data-testid={`${testid}-option-${o.code}`}
                    />
                    <span className="flex-1 text-gray-900">{o.name}</span>
                  </label>
                </li>
              );
            })}
          </ul>
          {(isAny || selected.length > 0) && (
            <div className="flex justify-end border-t border-surface-200 p-2">
              <button
                type="button"
                onClick={clear}
                className="text-xs text-gray-500 hover:text-gray-900"
                data-testid={`${testid}-clear`}
              >
                Очистить
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ChevronDown() {
  return (
    <svg
      className="h-4 w-4 shrink-0 text-gray-400"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="6 9 12 15 18 9" />
    </svg>
  );
}
