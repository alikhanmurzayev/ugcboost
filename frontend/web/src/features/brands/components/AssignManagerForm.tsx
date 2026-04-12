import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { assignManager } from "@/api/brands";
import { brandKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";

interface AssignManagerFormProps {
  brandId: string;
}

export default function AssignManagerForm({ brandId }: AssignManagerFormProps) {
  const { t } = useTranslation(["brands", "common"]);
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [tempPassword, setTempPassword] = useState("");

  const mutation = useMutation({
    mutationFn: (e: string) => assignManager(brandId, e),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: brandKeys.detail(brandId) });
      queryClient.invalidateQueries({ queryKey: brandKeys.all() });
      setEmail("");
      setError("");
      if (res.data.tempPassword) {
        setTempPassword(res.data.tempPassword);
      }
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : t("brands:assignError"));
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!email.trim()) {
      setError(t("common:emailRequired"));
      return;
    }
    setError("");
    mutation.mutate(email.trim());
  }

  return (
    <div className="mt-4 border-t border-surface-200 pt-4">
      <form onSubmit={handleSubmit} className="flex items-end gap-3">
        <div className="flex-1">
          <label className="mb-1 block text-sm font-medium text-gray-700">{t("brands:managerEmail")}</label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="manager@example.com"
            className="w-full rounded-button border border-surface-300 px-3 py-2 text-sm"
            data-testid="assign-manager-input"
          />
        </div>
        <button
          type="submit"
          disabled={mutation.isPending}
          className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:opacity-50"
          data-testid="assign-manager-submit"
        >
          {mutation.isPending ? t("brands:assigning") : t("brands:assign")}
        </button>
      </form>
      {error && <p className="mt-2 text-sm text-red-600" role="alert">{error}</p>}

      {tempPassword && (
        <div className="mt-3 rounded-button bg-green-50 p-3 text-sm">
          <p className="font-medium text-green-800">{t("brands:tempPassword")}</p>
          <p className="mt-1 font-mono text-green-900">{tempPassword}</p>
          <button onClick={() => setTempPassword("")} className="mt-2 text-xs text-green-600 hover:underline">
            {t("common:hide")}
          </button>
        </div>
      )}
    </div>
  );
}
