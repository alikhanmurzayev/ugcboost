import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { approveApplication } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";

interface ApproveApplicationDialogProps {
  applicationId: string;
  onApiError: (message: string) => void;
  onCloseDrawer: () => void;
}

export default function ApproveApplicationDialog({
  applicationId,
  onApiError,
  onCloseDrawer,
}: ApproveApplicationDialogProps) {
  const { t } = useTranslation("creatorApplications");
  const { t: tCommon } = useTranslation("common");
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [inlineError, setInlineError] = useState("");

  const mutation = useMutation({
    mutationFn: () => approveApplication(applicationId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
      setOpen(false);
      onCloseDrawer();
    },
    onError: (err) => {
      const isApi = err instanceof ApiError;
      const isClient4xx =
        isApi && err.status >= 400 && err.status < 500 && err.code !== "INTERNAL_ERROR";

      if (isApi && isClient4xx) {
        queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
        onApiError(getErrorMessage(err.code));
        if (err.status === 404) {
          onCloseDrawer();
        }
        setOpen(false);
      } else {
        setInlineError(t("approveDialog.retryError"));
      }
    },
    onSettled: () => {
      setIsSubmitting(false);
    },
  });

  const isPending = mutation.isPending || isSubmitting;

  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      e.stopImmediatePropagation();
      if (!isPending) setOpen(false);
    }
    window.addEventListener("keydown", handleKey, true);
    return () => window.removeEventListener("keydown", handleKey, true);
  }, [open, isPending]);

  function handleOpen() {
    setInlineError("");
    setOpen(true);
  }

  function handleClose() {
    if (isPending) return;
    setOpen(false);
  }

  function handleSubmit() {
    if (isPending) return;
    setIsSubmitting(true);
    setInlineError("");
    mutation.mutate();
  }

  return (
    <>
      <button
        type="button"
        onClick={handleOpen}
        data-testid="approve-button"
        className="rounded-button border border-emerald-600 px-4 py-2 text-sm font-semibold text-emerald-700 transition hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {t("actions.approve")}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center p-4"
          data-testid="approve-confirm-dialog"
        >
          <button
            type="button"
            aria-label={tCommon("cancel")}
            onClick={handleClose}
            disabled={isPending}
            data-testid="approve-confirm-backdrop"
            className="absolute inset-0 cursor-default bg-black/40 focus:outline-none disabled:cursor-not-allowed"
          />
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="approve-confirm-title"
            className="relative z-10 w-[420px] max-w-full rounded-card bg-white p-5 shadow-xl"
          >
            <h2
              id="approve-confirm-title"
              className="text-base font-semibold text-gray-900"
            >
              {t("approveDialog.title")}
            </h2>
            <p className="mt-3 text-sm text-gray-700">
              {t("approveDialog.body")}
            </p>
            {inlineError && (
              <p
                className="mt-3 text-sm text-red-600"
                role="alert"
                data-testid="approve-dialog-error"
              >
                {inlineError}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                onClick={handleClose}
                disabled={isPending}
                data-testid="approve-confirm-cancel"
                className="rounded-button px-4 py-2 text-sm text-gray-600 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {tCommon("cancel")}
              </button>
              <button
                type="button"
                onClick={handleSubmit}
                disabled={isPending}
                data-testid="approve-confirm-submit"
                className="rounded-button bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isPending ? t("actions.approving") : t("approveDialog.submit")}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
