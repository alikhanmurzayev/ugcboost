import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { rejectApplication } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";

interface RejectApplicationDialogProps {
  applicationId: string;
  hasTelegram: boolean;
  onApiError: (message: string) => void;
  onCloseDrawer: () => void;
}

export default function RejectApplicationDialog({
  applicationId,
  hasTelegram,
  onApiError,
  onCloseDrawer,
}: RejectApplicationDialogProps) {
  const { t } = useTranslation("creatorApplications");
  const { t: tCommon } = useTranslation("common");
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [inlineError, setInlineError] = useState("");

  const mutation = useMutation({
    mutationFn: () => rejectApplication(applicationId),
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
        setInlineError(t("rejectDialog.retryError"));
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
        data-testid="reject-button"
        className="rounded-button border border-red-600 px-4 py-2 text-sm font-semibold text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {t("actions.reject")}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center p-4"
          data-testid="reject-confirm-dialog"
        >
          <button
            type="button"
            aria-label={tCommon("cancel")}
            onClick={handleClose}
            disabled={isPending}
            data-testid="reject-confirm-backdrop"
            className="absolute inset-0 cursor-default bg-black/40 focus:outline-none disabled:cursor-not-allowed"
          />
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="reject-confirm-title"
            className="relative z-10 w-[420px] max-w-full rounded-card bg-white p-5 shadow-xl"
          >
            <h2
              id="reject-confirm-title"
              className="text-base font-semibold text-gray-900"
            >
              {t("rejectDialog.title")}
            </h2>
            <p className="mt-3 text-sm text-gray-700">
              {hasTelegram ? t("rejectDialog.body") : t("rejectDialog.bodyNoTg")}
            </p>
            {inlineError && (
              <p
                className="mt-3 text-sm text-red-600"
                role="alert"
                data-testid="reject-dialog-error"
              >
                {inlineError}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                onClick={handleClose}
                disabled={isPending}
                data-testid="reject-confirm-cancel"
                className="rounded-button px-4 py-2 text-sm text-gray-600 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {tCommon("cancel")}
              </button>
              <button
                type="button"
                onClick={handleSubmit}
                disabled={isPending}
                data-testid="reject-confirm-submit"
                className="rounded-button bg-red-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isPending ? t("actions.rejecting") : t("rejectDialog.submit")}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
