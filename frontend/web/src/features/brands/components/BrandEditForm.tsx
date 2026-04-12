import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { updateBrand } from "@/api/brands";
import { brandKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";

interface BrandEditFormProps {
  brandId: string;
  currentName: string;
  onClose: () => void;
}

export default function BrandEditForm({ brandId, currentName, onClose }: BrandEditFormProps) {
  const { t } = useTranslation(["brands", "common"]);
  const queryClient = useQueryClient();
  const [name, setName] = useState(currentName);
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: (n: string) => updateBrand(brandId, n),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: brandKeys.detail(brandId) });
      queryClient.invalidateQueries({ queryKey: brandKeys.all() });
      onClose();
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : t("brands:updateError"));
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      setError(t("common:nameRequired"));
      return;
    }
    setError("");
    mutation.mutate(name.trim());
  }

  return (
    <>
      <form onSubmit={handleSubmit} className="flex items-end gap-3">
        <div className="flex-1">
          <label className="mb-1 block text-sm font-medium text-gray-700">{t("brands:name")}</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded-button border border-surface-300 px-3 py-2 text-sm"
            data-testid="brand-name-input"
            autoFocus
          />
        </div>
        <button
          type="submit"
          disabled={mutation.isPending}
          className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:opacity-50"
          data-testid="edit-brand-submit"
        >
          {mutation.isPending ? t("common:saving") : t("common:save")}
        </button>
        <button
          type="button"
          onClick={onClose}
          className="rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
        >
          {t("common:cancel")}
        </button>
      </form>
      {error && <p className="mt-2 text-sm text-red-600" role="alert">{error}</p>}
    </>
  );
}
