import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { components } from "@/api/generated/schema";
import { verifyApplicationSocialManually } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import type { SocialPlatform } from "../types";

type DetailSocial = components["schemas"]["CreatorApplicationDetailSocial"];

const PLATFORM_LABELS: Record<SocialPlatform, string> = {
  instagram: "Instagram",
  tiktok: "TikTok",
  threads: "Threads",
};

interface VerifyManualDialogProps {
  open: boolean;
  applicationId: string;
  social: DetailSocial;
  onClose: () => void;
  onCloseDrawer: () => void;
  onApiError: (message: string) => void;
}

export default function VerifyManualDialog({
  open,
  applicationId,
  social,
  onClose,
  onCloseDrawer,
  onApiError,
}: VerifyManualDialogProps) {
  const { t } = useTranslation("creatorApplications");
  const queryClient = useQueryClient();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [inlineError, setInlineError] = useState("");

  const mutation = useMutation({
    mutationFn: () => verifyApplicationSocialManually(applicationId, social.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
      onClose();
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
        onClose();
      } else {
        setInlineError(t("verifyDialog.retryError"));
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
      if (e.key === "Escape" && !isPending) {
        e.stopImmediatePropagation();
        onClose();
      }
    }
    window.addEventListener("keydown", handleKey, true);
    return () => window.removeEventListener("keydown", handleKey, true);
  }, [open, isPending, onClose]);

  if (!open) return null;

  function handleSubmit() {
    if (isPending) return;
    setIsSubmitting(true);
    setInlineError("");
    mutation.mutate();
  }

  function handleClose() {
    if (isPending) return;
    onClose();
  }

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center p-4"
      data-testid="verify-confirm-dialog"
    >
      <button
        type="button"
        aria-label={t("verifyDialog.cancel")}
        onClick={handleClose}
        disabled={isPending}
        data-testid="verify-confirm-backdrop"
        className="absolute inset-0 cursor-default bg-black/40 focus:outline-none disabled:cursor-not-allowed"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="verify-confirm-title"
        className="relative z-10 w-[420px] max-w-full rounded-card bg-white p-5 shadow-xl"
      >
        <h2
          id="verify-confirm-title"
          className="text-base font-semibold text-gray-900"
        >
          {t("verifyDialog.title")}
        </h2>
        <p className="mt-3 text-sm text-gray-700">
          {t("verifyDialog.body", {
            handle: social.handle,
            platform: PLATFORM_LABELS[social.platform],
          })}
        </p>
        {inlineError && (
          <p
            className="mt-3 text-sm text-red-600"
            role="alert"
            data-testid="verify-dialog-error"
          >
            {inlineError}
          </p>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={handleClose}
            disabled={isPending}
            data-testid="verify-confirm-cancel"
            className="rounded-button px-4 py-2 text-sm text-gray-600 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {t("verifyDialog.cancel")}
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={isPending}
            data-testid="verify-confirm-submit"
            className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isPending ? t("actions.verifying") : t("verifyDialog.submit")}
          </button>
        </div>
      </div>
    </div>
  );
}
