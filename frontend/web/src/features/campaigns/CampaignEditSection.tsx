import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  updateCampaign,
  type Campaign,
  type CampaignInput,
} from "@/api/campaigns";
import { ApiError } from "@/api/client";
import { campaignKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";

const NAME_MAX_LEN = 255;
const TMA_URL_MAX_LEN = 2048;

interface CampaignEditSectionProps {
  campaign: Campaign;
  onCancel: () => void;
  onSaved: () => void;
}

export default function CampaignEditSection({
  campaign,
  onCancel,
  onSaved,
}: CampaignEditSectionProps) {
  const { t } = useTranslation(["campaigns", "common"]);
  const queryClient = useQueryClient();

  const [name, setName] = useState(campaign.name);
  const [tmaUrl, setTmaUrl] = useState(campaign.tmaUrl);
  const [nameError, setNameError] = useState("");
  const [tmaUrlError, setTmaUrlError] = useState("");
  const [formError, setFormError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { mutate, isPending } = useMutation({
    mutationFn: (input: CampaignInput) => updateCampaign(campaign.id, input),
    onSuccess() {
      void queryClient.invalidateQueries({
        queryKey: campaignKeys.detail(campaign.id),
      });
      void queryClient.invalidateQueries({ queryKey: campaignKeys.all() });
      setFormError("");
      onSaved();
    },
    onError(err) {
      if (err instanceof ApiError) {
        setFormError(getErrorMessage(err.code));
      } else {
        setFormError(t("common:errors.unknown"));
      }
    },
    onSettled() {
      setIsSubmitting(false);
    },
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (isSubmitting || isPending) return;
    setFormError("");

    const trimmedName = name.trim();
    const trimmedTmaUrl = tmaUrl.trim();

    let invalid = false;
    if (!trimmedName) {
      setNameError(t("campaigns:create.nameRequired"));
      invalid = true;
    } else if (trimmedName.length > NAME_MAX_LEN) {
      setNameError(t("common:errors.CAMPAIGN_NAME_TOO_LONG"));
      invalid = true;
    } else {
      setNameError("");
    }
    if (!trimmedTmaUrl) {
      setTmaUrlError(t("campaigns:create.tmaUrlRequired"));
      invalid = true;
    } else if (trimmedTmaUrl.length > TMA_URL_MAX_LEN) {
      setTmaUrlError(t("common:errors.CAMPAIGN_TMA_URL_TOO_LONG"));
      invalid = true;
    } else {
      setTmaUrlError("");
    }
    if (invalid) return;

    setIsSubmitting(true);
    mutate({ name: trimmedName, tmaUrl: trimmedTmaUrl });
  }

  const submitDisabled = isSubmitting || isPending;

  return (
    <form
      onSubmit={handleSubmit}
      className="mt-4 space-y-4"
      data-testid="campaign-edit-form"
      noValidate
    >
      <div>
        <label
          htmlFor="campaign-edit-name"
          className="mb-1 block text-sm font-medium text-gray-700"
        >
          {t("campaigns:create.nameLabel")}
        </label>
        <input
          id="campaign-edit-name"
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={NAME_MAX_LEN}
          placeholder={t("campaigns:create.namePlaceholder")}
          aria-describedby={nameError ? "campaign-edit-name-error" : undefined}
          aria-invalid={nameError ? true : undefined}
          className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
          data-testid="campaign-edit-name-input"
        />
        {nameError && (
          <p
            id="campaign-edit-name-error"
            role="alert"
            className="mt-1 text-sm text-red-600"
            data-testid="campaign-edit-name-error"
          >
            {nameError}
          </p>
        )}
      </div>

      <div>
        <label
          htmlFor="campaign-edit-tma-url"
          className="mb-1 block text-sm font-medium text-gray-700"
        >
          {t("campaigns:create.tmaUrlLabel")}
        </label>
        <input
          id="campaign-edit-tma-url"
          type="text"
          value={tmaUrl}
          onChange={(e) => setTmaUrl(e.target.value)}
          maxLength={TMA_URL_MAX_LEN}
          placeholder={t("campaigns:create.tmaUrlPlaceholder")}
          aria-describedby={
            tmaUrlError ? "campaign-edit-tma-url-error" : undefined
          }
          aria-invalid={tmaUrlError ? true : undefined}
          className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
          data-testid="campaign-edit-tma-url-input"
        />
        {tmaUrlError && (
          <p
            id="campaign-edit-tma-url-error"
            role="alert"
            className="mt-1 text-sm text-red-600"
            data-testid="campaign-edit-tma-url-error"
          >
            {tmaUrlError}
          </p>
        )}
      </div>

      {formError && (
        <p
          role="alert"
          className="text-sm text-red-600"
          data-testid="campaign-edit-error"
        >
          {formError}
        </p>
      )}

      <div className="flex items-center gap-3">
        <button
          type="submit"
          disabled={submitDisabled}
          className="rounded-button bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
          data-testid="campaign-edit-submit"
        >
          {submitDisabled
            ? t("campaigns:edit.submittingButton")
            : t("campaigns:edit.submitButton")}
        </button>
        <button
          type="button"
          onClick={onCancel}
          disabled={submitDisabled}
          className="rounded-button border border-surface-300 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-surface-200 disabled:opacity-50"
          data-testid="campaign-edit-cancel"
        >
          {t("campaigns:edit.cancelButton")}
        </button>
      </div>
    </form>
  );
}
