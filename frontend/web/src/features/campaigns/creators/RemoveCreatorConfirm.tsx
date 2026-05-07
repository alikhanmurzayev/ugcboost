import { useEffect } from "react";
import { useTranslation } from "react-i18next";

interface RemoveCreatorConfirmProps {
  open: boolean;
  creatorName: string;
  isLoading: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: () => void;
}

export default function RemoveCreatorConfirm({
  open,
  creatorName,
  isLoading,
  error,
  onClose,
  onConfirm,
}: RemoveCreatorConfirmProps) {
  const { t } = useTranslation("campaigns");

  useEffect(() => {
    if (!open) return;

    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !isLoading) onClose();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open, isLoading, onClose]);

  if (!open) return null;

  return (
    <>
      <button
        type="button"
        aria-label={t("campaignCreators.removeConfirmCloseAria")}
        onClick={onClose}
        disabled={isLoading}
        data-testid="remove-creator-confirm-backdrop"
        className="fixed inset-0 z-40 cursor-default bg-black/30 focus:outline-none disabled:cursor-not-allowed"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="remove-creator-confirm-title"
        className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-card border border-surface-300 bg-white p-6 shadow-xl outline-none"
        data-testid="remove-creator-confirm"
      >
        <h2
          id="remove-creator-confirm-title"
          className="text-lg font-semibold text-gray-900"
        >
          {t("campaignCreators.removeConfirmTitle")}
        </h2>
        <p className="mt-2 text-sm text-gray-700">
          {t("campaignCreators.removeConfirmMessage", { name: creatorName })}
        </p>
        {error && (
          <p
            className="mt-3 rounded-button border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            role="alert"
            data-testid="remove-creator-confirm-error"
          >
            {error}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={isLoading}
            className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
            data-testid="remove-creator-confirm-cancel"
          >
            {t("campaignCreators.cancelButton")}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={isLoading}
            className="rounded-button bg-red-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
            data-testid="remove-creator-confirm-submit"
          >
            {isLoading
              ? t("campaignCreators.removeSubmittingButton")
              : t("campaignCreators.removeConfirmButton")}
          </button>
        </div>
      </div>
    </>
  );
}
