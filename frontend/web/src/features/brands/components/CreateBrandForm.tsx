import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createBrand } from "@/api/brands";
import { brandKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";

interface CreateBrandFormProps {
  onClose: () => void;
}

export default function CreateBrandForm({ onClose }: CreateBrandFormProps) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: (n: string) => createBrand(n),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: brandKeys.all() });
      onClose();
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось создать бренд");
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      setError("Название обязательно");
      return;
    }
    setError("");
    mutation.mutate(name.trim());
  }

  return (
    <form onSubmit={handleSubmit} className="mt-4 flex items-end gap-3" data-testid="create-brand-form">
      <div className="flex-1">
        <label className="mb-1 block text-sm font-medium text-gray-700">Название</label>
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
        data-testid="create-brand-submit"
      >
        {mutation.isPending ? "Создание..." : "Создать"}
      </button>
      <button
        type="button"
        onClick={onClose}
        className="rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
      >
        Отмена
      </button>
      {error && <p className="text-sm text-red-600" role="alert">{error}</p>}
    </form>
  );
}
