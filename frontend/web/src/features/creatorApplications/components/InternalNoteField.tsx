import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { saveInternalNote } from "@/api/creatorApplications";
import { creatorApplicationKeys } from "@/shared/constants/queryKeys";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";
import type { Application } from "../types";

interface Props {
  application: Application;
}

// The caller is expected to remount this component with a fresh `key` when
// the application changes (e.g. via prev/next nav), so local form state stays
// in sync without manual effects.
export default function InternalNoteField({ application }: Props) {
  const { t } = useTranslation("creatorApplications");
  const queryClient = useQueryClient();
  const [value, setValue] = useState(application.internalNote ?? "");
  const [error, setError] = useState("");

  const mut = useMutation({
    mutationFn: () => saveInternalNote(application.id, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.saveNoteError"),
      );
    },
  });

  const dirty = value !== (application.internalNote ?? "");

  return (
    <div data-testid="internal-note-field">
      <label
        htmlFor={`internal-note-${application.id}`}
        className="block text-xs font-medium uppercase tracking-wide text-gray-500"
      >
        {t("internalNote.label")}
      </label>
      <textarea
        id={`internal-note-${application.id}`}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          if (error) setError("");
        }}
        rows={2}
        placeholder={t("internalNote.placeholder")}
        className="mt-1 w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
        data-testid="internal-note-input"
      />
      <div className="mt-2 flex items-center justify-end gap-3">
        {error && (
          <p className="text-sm text-red-600" role="alert">
            {error}
          </p>
        )}
        <button
          type="button"
          onClick={() => mut.mutate()}
          disabled={!dirty || mut.isPending}
          className="rounded-button bg-primary px-3 py-1.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
          data-testid="internal-note-save"
        >
          {mut.isPending ? t("internalNote.saving") : t("internalNote.save")}
        </button>
      </div>
    </div>
  );
}
