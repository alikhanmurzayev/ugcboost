import { useEffect, useMemo, useRef, useState } from "react";

interface Option {
  code: string;
  name: string;
}

interface Props {
  options: Option[];
  value?: string; // option code
  onChange: (code: string | undefined) => void;
  placeholder: string;
  searchPlaceholder?: string;
  isLoading?: boolean;
  searchable?: boolean;
  className?: string;
  testid?: string;
}

export default function Select({
  options,
  value,
  onChange,
  placeholder,
  searchPlaceholder = "Найти...",
  isLoading,
  searchable = true,
  className = "w-full",
  testid = "select",
}: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handlePointer(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setQuery("");
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setOpen(false);
        setQuery("");
      }
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

  const current = options.find((o) => o.code === value);

  function pick(code: string) {
    onChange(code);
    setOpen(false);
    setQuery("");
  }

  return (
    <div ref={ref} className={`relative inline-block ${className}`}>
      <button
        type="button"
        onClick={() => setOpen((p) => !p)}
        aria-haspopup="listbox"
        aria-expanded={open}
        data-testid={testid}
        className="inline-flex w-full items-center justify-between gap-2 rounded-button border border-gray-300 bg-white px-3 py-2 text-left text-sm transition hover:border-primary/40 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary-100"
      >
        <span className={current ? "text-gray-900" : "text-gray-500"}>
          {current?.name ?? placeholder}
        </span>
        <ChevronDown />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 w-full min-w-60 rounded-card border border-surface-300 bg-white shadow-lg">
          {searchable && (
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
          )}
          <ul
            className="max-h-56 overflow-y-auto py-1"
            role="listbox"
          >
            {isLoading && (
              <li className="px-3 py-2 text-sm text-gray-400">Загрузка...</li>
            )}
            {!isLoading && filtered.length === 0 && (
              <li className="px-3 py-2 text-sm text-gray-400">
                Ничего не найдено
              </li>
            )}
            {filtered.map((o) => (
              <li key={o.code}>
                <button
                  type="button"
                  onClick={() => pick(o.code)}
                  data-testid={`${testid}-option-${o.code}`}
                  className={`flex w-full cursor-pointer items-center px-3 py-2 text-left text-sm hover:bg-primary-50 ${
                    o.code === value ? "bg-primary-50 font-medium text-primary" : "text-gray-900"
                  }`}
                >
                  {o.name}
                </button>
              </li>
            ))}
          </ul>
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
