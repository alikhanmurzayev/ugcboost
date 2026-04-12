import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  getBrand,
  updateBrand,
  assignManager,
  removeManager,
} from "@/api/brands";
import { ROUTES } from "@/shared/constants/routes";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";

export default function BrandDetailPage() {
  const { brandId } = useParams<{ brandId: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState("");
  const [managerEmail, setManagerEmail] = useState("");
  const [tempPassword, setTempPassword] = useState("");
  const [error, setError] = useState("");
  const [removeConfirm, setRemoveConfirm] = useState<string | null>(null);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["brand", brandId],
    queryFn: () => getBrand(brandId as string),
    enabled: !!brandId,
  });

  const updateMut = useMutation({
    mutationFn: (name: string) => updateBrand(brandId as string, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["brand", brandId] });
      queryClient.invalidateQueries({ queryKey: ["brands"] });
      setEditing(false);
      setError("");
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось обновить бренд");
    },
  });

  const assignMut = useMutation({
    mutationFn: (email: string) => assignManager(brandId as string, email),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["brand", brandId] });
      queryClient.invalidateQueries({ queryKey: ["brands"] });
      setManagerEmail("");
      setError("");
      if (res.data.tempPassword) {
        setTempPassword(res.data.tempPassword);
      }
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось назначить менеджера");
    },
  });

  const removeMut = useMutation({
    mutationFn: (userId: string) => removeManager(brandId as string, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["brand", brandId] });
      queryClient.invalidateQueries({ queryKey: ["brands"] });
      setRemoveConfirm(null);
      setError("");
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось удалить менеджера");
      setRemoveConfirm(null);
    },
  });

  if (!brandId) return <ErrorState message="Бренд не найден" />;
  if (isLoading) return <Spinner className="mt-12" />;
  if (isError || !data) return <ErrorState message="Ошибка загрузки" onRetry={() => void refetch()} />;

  const brand = data.data;

  function startEdit() {
    setEditName(brand.name);
    setEditing(true);
  }

  function handleUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!editName.trim()) return;
    updateMut.mutate(editName.trim());
  }

  function handleAssign(e: React.FormEvent) {
    e.preventDefault();
    if (!managerEmail.trim()) return;
    assignMut.mutate(managerEmail.trim());
  }

  return (
    <div className="max-w-2xl">
      <button
        onClick={() => navigate("/" + ROUTES.BRANDS)}
        className="mb-4 text-sm text-primary hover:underline"
      >
        &larr; Назад к списку
      </button>

      {/* Brand info */}
      <div className="rounded-card border border-surface-300 bg-white p-6">
        {editing ? (
          <form onSubmit={handleUpdate} className="flex items-end gap-3">
            <div className="flex-1">
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Название
              </label>
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="w-full rounded-button border border-surface-300 px-3 py-2 text-sm"
                autoFocus
              />
            </div>
            <button
              type="submit"
              disabled={updateMut.isPending}
              className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:opacity-50"
            >
              {updateMut.isPending ? "Сохранение..." : "Сохранить"}
            </button>
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setError("");
              }}
              className="rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
            >
              Отмена
            </button>
          </form>
        ) : (
          <div className="flex items-center justify-between">
            <h1 className="text-2xl font-bold text-gray-900">{brand.name}</h1>
            <button
              onClick={startEdit}
              className="rounded-button border border-surface-300 px-3 py-1.5 text-sm text-gray-600 hover:bg-surface-200"
            >
              Изменить
            </button>
          </div>
        )}
        <p className="mt-2 text-sm text-gray-500">
          Создан: {new Date(brand.createdAt).toLocaleDateString("ru")}
        </p>
      </div>

      {error && <p className="mt-4 text-sm text-red-600" role="alert">{error}</p>}

      {/* Managers */}
      <div className="mt-6 rounded-card border border-surface-300 bg-white p-6">
        <h2 className="text-lg font-bold text-gray-900">Менеджеры</h2>

        {brand.managers.length === 0 ? (
          <p className="mt-3 text-sm text-gray-500">Нет назначенных менеджеров</p>
        ) : (
          <table className="mt-3 w-full text-left text-sm">
            <thead>
              <tr className="border-b border-surface-300 text-gray-500">
                <th scope="col" className="pb-2 font-medium">Email</th>
                <th scope="col" className="pb-2 font-medium">Назначен</th>
                <th scope="col" className="pb-2 font-medium" />
              </tr>
            </thead>
            <tbody>
              {brand.managers.map((m) => (
                <tr
                  key={m.userId}
                  className="border-b border-surface-200"
                >
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
        )}

        {/* Add manager */}
        <form
          onSubmit={handleAssign}
          className="mt-4 flex items-end gap-3 border-t border-surface-200 pt-4"
        >
          <div className="flex-1">
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Email менеджера
            </label>
            <input
              type="email"
              value={managerEmail}
              onChange={(e) => setManagerEmail(e.target.value)}
              placeholder="manager@example.com"
              className="w-full rounded-button border border-surface-300 px-3 py-2 text-sm"
            />
          </div>
          <button
            type="submit"
            disabled={assignMut.isPending}
            className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:opacity-50"
          >
            {assignMut.isPending ? "Назначение..." : "Назначить"}
          </button>
        </form>

        {tempPassword && (
          <div className="mt-3 rounded-button bg-green-50 p-3 text-sm">
            <p className="font-medium text-green-800">
              Новый менеджер создан. Временный пароль:
            </p>
            <p className="mt-1 font-mono text-green-900">{tempPassword}</p>
            <button
              onClick={() => setTempPassword("")}
              className="mt-2 text-xs text-green-600 hover:underline"
            >
              Скрыть
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
