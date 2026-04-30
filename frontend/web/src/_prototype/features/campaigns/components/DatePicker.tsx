import { useEffect, useRef, useState } from "react";
import { DayPicker } from "react-day-picker";
import { ru } from "react-day-picker/locale";
import "react-day-picker/style.css";

interface Props {
  value?: string; // ISO date "YYYY-MM-DD"
  onChange: (value: string | undefined) => void;
  placeholder?: string;
  minDate?: Date;
  testid?: string;
}

export default function DatePicker({
  value,
  onChange,
  placeholder = "Выберите дату",
  minDate,
  testid = "date-picker",
}: Props) {
  const [open, setOpen] = useState(false);
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

  const selected = value ? parseISO(value) : undefined;

  function handleSelect(next: Date | undefined) {
    onChange(next ? toISODate(next) : undefined);
    if (next) setOpen(false);
  }

  function clear() {
    onChange(undefined);
  }

  const display = selected ? formatDate(selected) : placeholder;

  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen((p) => !p)}
        className="inline-flex w-60 items-center justify-between gap-2 rounded-button border border-gray-300 bg-white px-3 py-2 text-sm transition hover:border-primary/40 hover:bg-surface-100 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary-100"
        aria-haspopup="dialog"
        aria-expanded={open}
        data-testid={testid}
      >
        <span className="flex items-center gap-2">
          <CalendarIcon />
          <span className={selected ? "text-gray-900" : "text-gray-500"}>
            {display}
          </span>
        </span>
        <ChevronDownIcon />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-2 rounded-card border border-surface-300 bg-white p-3 shadow-lg">
          <DayPicker
            mode="single"
            selected={selected}
            onSelect={handleSelect}
            locale={ru}
            ISOWeek
            disabled={minDate ? { before: minDate } : undefined}
            className="rdp-ugc"
          />
          {selected && (
            <div className="mt-2 flex justify-end border-t border-surface-200 pt-2">
              <button
                type="button"
                onClick={clear}
                className="text-sm text-gray-500 hover:text-gray-900"
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

function CalendarIcon() {
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
      <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
      <line x1="16" y1="2" x2="16" y2="6" />
      <line x1="8" y1="2" x2="8" y2="6" />
      <line x1="3" y1="10" x2="21" y2="10" />
    </svg>
  );
}

function ChevronDownIcon() {
  return (
    <svg
      className="h-4 w-4 text-gray-400"
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

function parseISO(v: string): Date {
  const [y, m, d] = v.split("-").map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

function toISODate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function formatDate(d: Date): string {
  return d.toLocaleDateString("ru", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}
