import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

interface Option {
  code: string;
  name: string;
}

interface Props {
  options: Option[];
  selected: string[];
  onChange: (codes: string[]) => void;
  placeholder: string;
  searchPlaceholder: string;
  isLoading?: boolean;
  testid?: string;
  triggerId?: string;
}

export default function SearchableMultiselect({
  options,
  selected,
  onChange,
  placeholder,
  searchPlaceholder,
  isLoading,
  testid = "multiselect",
  triggerId,
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

  function toggle(code: string) {
    if (selected.includes(code)) {
      onChange(selected.filter((c) => c !== code));
    } else {
      onChange([...selected, code]);
    }
  }

  function clear() {
    onChange([]);
  }

  const selectedOptions = options.filter((o) => selected.includes(o.code));
  const display: ReactNode =
    selectedOptions.length === 0
      ? placeholder
      : selectedOptions.length <= 2
        ? selectedOptions.map((o) => o.name).join(", ")
        : `${selectedOptions[0]?.name ?? ""} +${selectedOptions.length - 1}`;

  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        id={triggerId}
        onClick={() => setOpen((p) => !p)}
        aria-haspopup="listbox"
        aria-expanded={open}
        data-testid={testid}
        className="inline-flex w-64 items-center justify-between gap-2 rounded-button border border-gray-300 bg-white px-3 py-1.5 text-left text-sm text-gray-700 transition hover:bg-surface-100"
      >
        <span className={selected.length === 0 ? "text-gray-500" : "text-gray-900"}>
          {display}
        </span>
        <ChevronDown />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 w-64 rounded-card border border-surface-300 bg-white shadow-lg">
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
          <ul
            className="max-h-56 overflow-y-auto py-1"
            role="listbox"
            aria-multiselectable="true"
          >
            {isLoading && (
              <li className="px-3 py-2 text-sm text-gray-400">Загрузка...</li>
            )}
            {!isLoading && filtered.length === 0 && (
              <li className="px-3 py-2 text-sm text-gray-400">Ничего не найдено</li>
            )}
            {filtered.map((o) => {
              const isSelected = selected.includes(o.code);
              return (
                <li key={o.code}>
                  <label className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm hover:bg-primary-50">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggle(o.code)}
                      className="h-4 w-4 rounded border-gray-300 text-primary focus:ring-primary"
                      data-testid={`${testid}-option-${o.code}`}
                    />
                    <span className="flex-1 text-gray-900">{o.name}</span>
                  </label>
                </li>
              );
            })}
          </ul>
          {selected.length > 0 && (
            <div className="flex justify-end border-t border-surface-200 p-2">
              <button
                type="button"
                onClick={clear}
                className="text-xs text-gray-500 hover:text-gray-900"
                data-testid={`${testid}-clear`}
              >
                Очистить ({selected.length})
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
      className="h-4 w-4 text-gray-500"
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
