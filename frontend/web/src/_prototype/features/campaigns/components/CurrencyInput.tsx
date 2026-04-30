import { type ChangeEvent } from "react";

interface Props {
  value?: number;
  onChange: (value: number | undefined) => void;
  placeholder?: string;
  className?: string;
  testid?: string;
  ariaLabel?: string;
}

// Numeric input that displays integers grouped by thousands ("10 000")
// and a trailing "₸" suffix. Stores a plain number in state.
export default function CurrencyInput({
  value,
  onChange,
  placeholder,
  className,
  testid,
  ariaLabel,
}: Props) {
  function handleChange(e: ChangeEvent<HTMLInputElement>) {
    // Strip everything but digits
    const digits = e.target.value.replace(/\D/g, "");
    if (!digits) {
      onChange(undefined);
      return;
    }
    onChange(Number(digits));
  }

  const display = value === undefined ? "" : value.toLocaleString("ru-RU");

  return (
    <div className={`relative inline-block ${className ?? ""}`}>
      <input
        type="text"
        inputMode="numeric"
        value={display}
        onChange={handleChange}
        placeholder={placeholder}
        aria-label={ariaLabel}
        className="w-full rounded-button border border-gray-300 bg-white px-3 py-2 pr-8 text-sm tabular-nums outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
        data-testid={testid}
      />
      <span
        className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-500"
        aria-hidden="true"
      >
        ₸
      </span>
    </div>
  );
}
