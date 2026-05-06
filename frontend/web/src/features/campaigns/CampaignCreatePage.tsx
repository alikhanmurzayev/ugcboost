import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createCampaign, type CampaignInput } from "@/api/campaigns";
import { ApiError } from "@/api/client";
import { ROUTES } from "@/shared/constants/routes";
import { campaignKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";

const NAME_MAX_LEN = 255;
const TMA_URL_MAX_LEN = 2048;

export default function CampaignCreatePage() {
  const { t } = useTranslation(["campaigns", "common"]);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [tmaUrl, setTmaUrl] = useState("");
  const [nameError, setNameError] = useState("");
  const [tmaUrlError, setTmaUrlError] = useState("");
  const [formError, setFormError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { mutate, isPending } = useMutation({
    mutationFn: (input: CampaignInput) => createCampaign(input),
    onSuccess(res) {
      void queryClient.invalidateQueries({ queryKey: campaignKeys.all() });
      navigate(`/${ROUTES.CAMPAIGN_DETAIL(res.data.id)}`);
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
    } else {
      setNameError("");
    }
    if (!trimmedTmaUrl) {
      setTmaUrlError(t("campaigns:create.tmaUrlRequired"));
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
    <div data-testid="campaign-create-page">
      <div>
        <Link
          to={`/${ROUTES.CAMPAIGNS}`}
          className="text-sm text-gray-500 hover:text-gray-700"
          data-testid="campaign-create-back"
        >
          {t("campaigns:create.backToList")}
        </Link>
        <h1 className="mt-2 text-2xl font-bold text-gray-900">
          {t("campaigns:create.title")}
        </h1>
        <p className="mt-1 text-sm text-gray-500">
          {t("campaigns:create.description")}
        </p>
      </div>

      <form
        onSubmit={handleSubmit}
        className="mt-6 max-w-lg space-y-4"
        data-testid="campaign-create-form"
        noValidate
      >
        <div>
          <label
            htmlFor="campaign-name"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            {t("campaigns:create.nameLabel")}
          </label>
          <input
            id="campaign-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={NAME_MAX_LEN}
            placeholder={t("campaigns:create.namePlaceholder")}
            aria-describedby={nameError ? "campaign-name-error" : undefined}
            aria-invalid={nameError ? true : undefined}
            className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
            data-testid="campaign-name-input"
          />
          {nameError && (
            <p
              id="campaign-name-error"
              role="alert"
              className="mt-1 text-sm text-red-600"
              data-testid="campaign-name-error"
            >
              {nameError}
            </p>
          )}
        </div>

        <div>
          <label
            htmlFor="campaign-tma-url"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            {t("campaigns:create.tmaUrlLabel")}
          </label>
          <input
            id="campaign-tma-url"
            type="text"
            value={tmaUrl}
            onChange={(e) => setTmaUrl(e.target.value)}
            maxLength={TMA_URL_MAX_LEN}
            placeholder={t("campaigns:create.tmaUrlPlaceholder")}
            aria-describedby={
              tmaUrlError ? "campaign-tma-url-error" : undefined
            }
            aria-invalid={tmaUrlError ? true : undefined}
            className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
            data-testid="campaign-tma-url-input"
          />
          {tmaUrlError && (
            <p
              id="campaign-tma-url-error"
              role="alert"
              className="mt-1 text-sm text-red-600"
              data-testid="campaign-tma-url-error"
            >
              {tmaUrlError}
            </p>
          )}
        </div>

        {formError && (
          <p
            role="alert"
            className="text-sm text-red-600"
            data-testid="create-campaign-error"
          >
            {formError}
          </p>
        )}

        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={submitDisabled}
            className="rounded-button bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
            data-testid="create-campaign-submit"
          >
            {submitDisabled
              ? t("campaigns:create.submittingButton")
              : t("campaigns:create.submitButton")}
          </button>
        </div>
      </form>
    </div>
  );
}
