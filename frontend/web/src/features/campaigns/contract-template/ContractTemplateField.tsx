import { useRef, useState, type ChangeEvent } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/api/client";
import {
  triggerDownloadContractTemplate,
  useUploadContractTemplate,
} from "./useContractTemplate";

interface Props {
  campaignId: string;
  campaignName: string;
  hasTemplate: boolean;
  disabled?: boolean;
}

export default function ContractTemplateField({
  campaignId,
  campaignName,
  hasTemplate,
  disabled = false,
}: Props) {
  const { t } = useTranslation("campaigns");
  const inputRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState("");
  const [isDownloading, setIsDownloading] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const upload = useUploadContractTemplate(campaignId);

  function openFilePicker() {
    setError("");
    inputRef.current?.click();
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-upload of the same file name
    if (!file) return;
    // Reject obvious non-PDFs before paying for an upload roundtrip — saves
    // bandwidth on a 50 MB .docx renamed to .pdf and gives the admin
    // immediate feedback instead of a 422 from the parser. Server-side
    // CONTRACT_INVALID_PDF still backs this up.
    const looksLikePDF =
      file.type === "application/pdf" ||
      file.name.toLowerCase().endsWith(".pdf");
    if (!looksLikePDF) {
      setError(t("contractTemplate.errorInvalidPDF"));
      return;
    }
    setIsSubmitting(true);
    setError("");
    upload.mutate(file, {
      onError(err) {
        if (err instanceof ApiError && err.serverMessage) {
          setError(err.serverMessage);
        } else {
          setError(t("contractTemplate.networkError"));
        }
      },
      onSettled() {
        setIsSubmitting(false);
      },
    });
  }

  async function handleDownload() {
    if (isDownloading) return;
    setIsDownloading(true);
    setError("");
    try {
      const safeName = campaignName.replace(/[^\p{L}\p{N}_-]+/gu, "-");
      await triggerDownloadContractTemplate(
        campaignId,
        `${safeName || "contract"}-template.pdf`,
      );
    } catch (err) {
      if (err instanceof ApiError && err.serverMessage) {
        setError(err.serverMessage);
      } else {
        setError(t("contractTemplate.networkError"));
      }
    } finally {
      setIsDownloading(false);
    }
  }

  const submitDisabled = disabled || isSubmitting || upload.isPending;

  return (
    <section
      className="mt-6 rounded-card border border-surface-300 bg-white p-6"
      data-testid="contract-template-section"
    >
      <h2 className="text-lg font-bold text-gray-900">
        {t("contractTemplate.sectionTitle")}
      </h2>
      <p className="mt-2 text-sm text-gray-600">
        {t("contractTemplate.descriptionEmpty")}
      </p>

      <input
        ref={inputRef}
        type="file"
        accept="application/pdf,.pdf"
        className="hidden"
        aria-label={t("contractTemplate.label")}
        onChange={handleFileChange}
        data-testid="contract-template-input"
      />

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={openFilePicker}
          disabled={submitDisabled}
          className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid={
            hasTemplate
              ? "contract-template-replace-button"
              : "contract-template-upload-button"
          }
        >
          {submitDisabled
            ? t("contractTemplate.loading")
            : hasTemplate
              ? t("contractTemplate.replaceButton")
              : t("contractTemplate.uploadButton")}
        </button>

        {hasTemplate && (
          <button
            type="button"
            onClick={handleDownload}
            disabled={disabled || isDownloading}
            className="rounded-button border border-surface-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
            data-testid="contract-template-download-button"
          >
            {t("contractTemplate.downloadButton")}
          </button>
        )}
      </div>

      {submitDisabled && (
        <p
          className="mt-3 text-sm text-gray-500"
          data-testid="contract-template-loading"
        >
          {t("contractTemplate.loading")}
        </p>
      )}

      {upload.isSuccess && upload.data && (
        <div
          className="mt-4 rounded-card border border-surface-300 bg-surface-100 p-3"
          data-testid="contract-template-placeholders"
        >
          <p className="text-sm font-medium text-gray-700">
            {t("contractTemplate.placeholdersFound")}
          </p>
          <ul className="mt-2 flex flex-wrap gap-2">
            {upload.data.data.placeholders.map((name) => (
              <li
                key={name}
                className="rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary"
                data-testid={`contract-template-placeholder-${name}`}
              >
                {name}
              </li>
            ))}
          </ul>
        </div>
      )}

      {error && (
        <p
          role="alert"
          className="mt-3 text-sm text-red-600"
          data-testid="contract-template-error"
        >
          {error}
        </p>
      )}
    </section>
  );
}
