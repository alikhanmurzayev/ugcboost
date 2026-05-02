import { useEffect, useRef, useState } from "react";
import { DayPicker, type DateRange } from "react-day-picker";
import { ru } from "react-day-picker/locale";
import "react-day-picker/style.css";

interface Props {
  from?: string;
  to?: string;
  onChange: (from?: string, to?: string) => void;
  placeholder?: string;
  testid?: string;
}

export default function DateRangePicker({
  from,
  to,
  onChange,
  placeholder = "Выбрать период",
  testid = "date-range-picker",
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

  const range: DateRange | undefined =
    from || to
      ? {
          from: from ? parseISO(from) : undefined,
          to: to ? parseISO(to) : undefined,
        }
      : undefined;

  function handleSelect(next: DateRange | undefined) {
    onChange(
      next?.from ? toISODate(next.from) : undefined,
      next?.to ? toISODate(next.to) : undefined,
    );
  }

  function clear() {
    onChange(undefined, undefined);
  }

  const display = formatRange(from, to) || placeholder;
  const hasValue = !!(from || to);

  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen((p) => !p)}
        className="inline-flex items-center gap-2 rounded-button border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 transition hover:bg-surface-100"
        aria-haspopup="dialog"
        aria-expanded={open}
        data-testid={testid}
      >
        <CalendarIcon />
        <span className={hasValue ? "text-gray-900" : "text-gray-500"}>
          {display}
        </span>
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 rounded-card border border-surface-300 bg-white p-3 shadow-lg">
          <DayPicker
            mode="range"
            selected={range}
            onSelect={handleSelect}
            locale={ru}
            ISOWeek
            className="rdp-ugc"
          />
          <div className="mt-2 flex justify-end gap-2 border-t border-surface-200 pt-2">
            {hasValue && (
              <button
                type="button"
                onClick={clear}
                className="text-sm text-gray-500 hover:text-gray-900"
                data-testid={`${testid}-clear`}
              >
                Очистить
              </button>
            )}
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="text-sm font-medium text-primary hover:underline"
              data-testid={`${testid}-done`}
            >
              Готово
            </button>
          </div>
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

function parseISO(value: string): Date {
  const [y, m, d] = value.split("-").map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

function toISODate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function formatRange(from?: string, to?: string): string {
  if (!from && !to) return "";
  const fmt = (s?: string) =>
    s
      ? parseISO(s).toLocaleDateString("ru", {
          day: "numeric",
          month: "short",
          year: "2-digit",
        })
      : "?";
  return `${fmt(from)} — ${fmt(to)}`;
}
