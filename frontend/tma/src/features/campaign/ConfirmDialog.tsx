import type { ReactNode } from "react";

type Variant = "primary" | "secondary";

export function ConfirmDialog({
  title,
  description,
  confirmText,
  cancelText,
  confirmVariant,
  onConfirm,
  onCancel,
  testIdPrefix,
}: {
  title: string;
  description: ReactNode;
  confirmText: string;
  cancelText: string;
  confirmVariant: Variant;
  onConfirm: () => void;
  onCancel: () => void;
  testIdPrefix?: string;
}) {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4 py-6"
    >
      <div className="w-full max-w-sm rounded-card bg-white p-6 shadow-2xl">
        <h2 id="confirm-title" className="text-lg font-bold text-gray-900">
          {title}
        </h2>
        <div className="mt-3 text-sm leading-relaxed text-gray-700">
          {description}
        </div>
        <div className="mt-6 flex flex-col gap-3">
          <button
            type="button"
            onClick={onConfirm}
            className={
              "w-full rounded-button py-3 text-base font-semibold transition-colors " +
              (confirmVariant === "primary"
                ? "bg-primary text-white hover:bg-primary-600 active:bg-primary-700"
                : "border border-surface-300 bg-surface-50 text-gray-900 hover:bg-surface-200")
            }
            data-testid={
              testIdPrefix ? `${testIdPrefix}-confirm` : undefined
            }
          >
            {confirmText}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="w-full rounded-button py-3 text-base font-medium text-gray-600 hover:text-gray-900"
            data-testid={
              testIdPrefix ? `${testIdPrefix}-cancel` : undefined
            }
          >
            {cancelText}
          </button>
        </div>
      </div>
    </div>
  );
}
