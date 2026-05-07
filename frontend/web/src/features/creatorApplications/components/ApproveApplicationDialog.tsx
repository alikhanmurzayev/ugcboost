import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { approveApplication } from "@/api/creatorApplications";
import { listCampaigns } from "@/api/campaigns";
import { ApiError } from "@/api/client";
import {
  campaignCreatorKeys,
  campaignKeys,
  creatorApplicationKeys,
  creatorKeys,
} from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import SearchableMultiselect from "@/shared/components/SearchableMultiselect";

interface ApproveApplicationDialogProps {
  applicationId: string;
  onApiError: (message: string) => void;
  onCloseDrawer: () => void;
}

const CAMPAIGNS_QUERY_PARAMS = {
  page: 1,
  perPage: 100,
  sort: "name" as const,
  order: "asc" as const,
  isDeleted: false,
};

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
  const [selectedCampaignIds, setSelectedCampaignIds] = useState<string[]>([]);

  const campaignsQuery = useQuery({
    queryKey: campaignKeys.list(CAMPAIGNS_QUERY_PARAMS),
    queryFn: () => listCampaigns(CAMPAIGNS_QUERY_PARAMS),
    enabled: open,
  });

  const mutation = useMutation({
    mutationFn: () => approveApplication(applicationId, selectedCampaignIds),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
      // creator was just materialised; "Все креаторы" must reflect it.
      queryClient.invalidateQueries({ queryKey: creatorKeys.all() });
      for (const campaignId of selectedCampaignIds) {
        queryClient.invalidateQueries({ queryKey: campaignKeys.detail(campaignId) });
        queryClient.invalidateQueries({ queryKey: campaignCreatorKeys.list(campaignId) });
      }
      setOpen(false);
      onCloseDrawer();
    },
    onError: (err) => {
      const isApi = err instanceof ApiError;
      const isClient4xx =
        isApi && err.status >= 400 && err.status < 500 && err.code !== "INTERNAL_ERROR";

      if (isApi && isClient4xx) {
        // Input-validation errors raised in the handler before any state
        // change AND mid-cycle add failures (creator already approved, but
        // some campaigns left unattached) — admin fixes the campaign list
        // inside the dialog and retries / fixes manually, so we keep the
        // dialog open with the actionable backend message inline.
        const isInlineDialogError =
          err.code === "CAMPAIGN_IDS_TOO_MANY" ||
          err.code === "CAMPAIGN_IDS_DUPLICATES" ||
          err.code === "CAMPAIGN_NOT_AVAILABLE_FOR_ADD" ||
          err.code === "CAMPAIGN_ADD_AFTER_APPROVE_FAILED";

        if (isInlineDialogError) {
          // For the post-approve failure, the creator is already created and
          // some campaigns may already be attached — invalidate downstream
          // caches so the rest of the UI does not lie about state.
          if (err.code === "CAMPAIGN_ADD_AFTER_APPROVE_FAILED") {
            queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
            queryClient.invalidateQueries({ queryKey: creatorKeys.all() });
            for (const campaignId of selectedCampaignIds) {
              queryClient.invalidateQueries({ queryKey: campaignKeys.detail(campaignId) });
              queryClient.invalidateQueries({ queryKey: campaignCreatorKeys.list(campaignId) });
            }
          }
          setInlineError(err.serverMessage ?? getErrorMessage(err.code));
          return;
        }

        // Aggregate-level 4xx (state changed, telegram missing, creator
        // already exists). Surface the localized message at the drawer level
        // and close the dialog so the admin re-evaluates the application.
        // 404 also tears down the drawer (the aggregate no longer exists).
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
  const isCampaignsLoading = open && campaignsQuery.isLoading;
  const submitDisabled = isPending || isCampaignsLoading;

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
    setSelectedCampaignIds([]);
    setOpen(true);
  }

  function handleClose() {
    if (isPending) return;
    setOpen(false);
  }

  function handleSubmit() {
    if (submitDisabled) return;
    setIsSubmitting(true);
    setInlineError("");
    mutation.mutate();
  }

  const campaignOptions = useMemo(
    () =>
      (campaignsQuery.data?.data.items ?? []).map((c) => ({
        code: c.id,
        name: c.name,
      })),
    [campaignsQuery.data],
  );

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
            className="relative z-10 w-[460px] max-w-full rounded-card bg-white p-5 shadow-xl"
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

            <div className="mt-4">
              <label
                htmlFor="approve-campaigns-multiselect"
                className="block text-sm font-medium text-gray-800"
              >
                {t("approveDialog.campaignsLabel")}
              </label>
              <p className="mt-1 text-xs text-gray-500">
                {t("approveDialog.campaignsHint")}
              </p>
              <div className="mt-2" id="approve-campaigns-multiselect">
                <SearchableMultiselect
                  options={campaignOptions}
                  selected={selectedCampaignIds}
                  onChange={setSelectedCampaignIds}
                  placeholder={
                    isCampaignsLoading
                      ? t("approveDialog.campaignsLoading")
                      : t("approveDialog.campaignsPlaceholder")
                  }
                  searchPlaceholder={t("approveDialog.campaignsSearchPlaceholder")}
                  isLoading={isCampaignsLoading}
                  testid="approve-campaigns-multiselect"
                />
              </div>
              {campaignsQuery.isError && (
                <p
                  className="mt-2 text-xs text-red-600"
                  role="alert"
                  data-testid="approve-dialog-campaigns-error"
                >
                  {t("approveDialog.campaignsLoadError")}
                </p>
              )}
            </div>

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
                disabled={submitDisabled}
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
