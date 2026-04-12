import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { removeManager } from "@/api/brands";
import type { ManagerInfo } from "@/api/brands";
import { brandKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";

interface ManagerListProps {
  brandId: string;
  managers: ManagerInfo[];
}

export default function ManagerList({ brandId, managers }: ManagerListProps) {
  const queryClient = useQueryClient();
  const [removeConfirm, setRemoveConfirm] = useState<string | null>(null);
  const [error, setError] = useState("");

  const removeMut = useMutation({
    mutationFn: (userId: string) => removeManager(brandId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: brandKeys.detail(brandId) });
      queryClient.invalidateQueries({ queryKey: brandKeys.all() });
      setRemoveConfirm(null);
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось удалить менеджера");
      setRemoveConfirm(null);
    },
  });

  if (managers.length === 0) {
    return <p className="mt-3 text-sm text-gray-500">Нет назначенных менеджеров</p>;
  }

  return (
    <>
      <table className="mt-3 w-full text-left text-sm" data-testid="managers-table">
        <thead>
          <tr className="border-b border-surface-300 text-gray-500">
            <th scope="col" className="pb-2 font-medium">Email</th>
            <th scope="col" className="pb-2 font-medium">Назначен</th>
            <th scope="col" className="pb-2 font-medium" />
          </tr>
        </thead>
        <tbody>
          {managers.map((m) => (
            <tr key={m.userId} className="border-b border-surface-200" data-testid={`manager-row-${m.userId}`}>
              <td className="py-2 text-gray-900">{m.email}</td>
              <td className="py-2 text-gray-500">
                {new Date(m.assignedAt).toLocaleDateString("ru")}
              </td>
              <td className="py-2 text-right">
                {removeConfirm === m.userId ? (
                  <span className="space-x-2">
                    <button
                      onClick={() => removeMut.mutate(m.userId)}
                      disabled={removeMut.isPending}
                      className="text-red-600 font-medium hover:text-red-800 disabled:opacity-50"
                    >
                      {removeMut.isPending ? "Удаление..." : "Да, удалить"}
                    </button>
                    <button
                      onClick={() => setRemoveConfirm(null)}
                      className="text-gray-500 hover:text-gray-700"
                    >
                      Отмена
                    </button>
                  </span>
                ) : (
                  <button
                    onClick={() => setRemoveConfirm(m.userId)}
                    className="text-red-500 hover:text-red-700"
                  >
                    Удалить
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {error && <p className="mt-2 text-sm text-red-600" role="alert">{error}</p>}
    </>
  );
}
