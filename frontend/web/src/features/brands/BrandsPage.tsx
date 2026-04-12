import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { listBrands, createBrand, deleteBrand } from "@/api/brands";
import { ROUTES } from "@/shared/constants/routes";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";

export default function BrandsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [error, setError] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["brands"],
    queryFn: () => listBrands(),
  });

  const createMut = useMutation({
    mutationFn: (name: string) => createBrand(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["brands"] });
      setShowCreate(false);
      setNewName("");
      setError("");
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось создать бренд");
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteBrand(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["brands"] });
      setDeleteConfirm(null);
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось удалить бренд");
      setDeleteConfirm(null);
    },
  });

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    createMut.mutate(newName.trim());
  }

  const brands = data?.data.brands ?? [];

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Бренды</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90"
        >
          Создать бренд
        </button>
      </div>

      {showCreate && (
        <form onSubmit={handleCreate} className="mt-4 flex items-end gap-3">
          <div className="flex-1">
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Название
            </label>
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="w-full rounded-button border border-surface-300 px-3 py-2 text-sm"
              autoFocus
            />
          </div>
          <button
            type="submit"
            disabled={createMut.isPending}
            className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90 disabled:opacity-50"
          >
            {createMut.isPending ? "Создание..." : "Создать"}
          </button>
          <button
            type="button"
            onClick={() => {
              setShowCreate(false);
              setNewName("");
              setError("");
            }}
            className="rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
          >
            Отмена
          </button>
          {error && <p className="text-sm text-red-600" role="alert">{error}</p>}
        </form>
      )}

      {isLoading ? (
        <Spinner className="mt-6" />
      ) : isError ? (
        <ErrorState message="Не удалось загрузить бренды" onRetry={() => void refetch()} />
      ) : brands.length === 0 ? (
        <p className="mt-6 text-gray-500">Нет брендов</p>
      ) : (
        <table className="mt-6 w-full text-left text-sm">
          <thead>
            <tr className="border-b border-surface-300 text-gray-500">
              <th scope="col" className="pb-2 font-medium">Название</th>
              <th scope="col" className="pb-2 font-medium">Менеджеры</th>
              <th scope="col" className="pb-2 font-medium">Создан</th>
              <th scope="col" className="pb-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {brands.map((b) => (
              <tr
                key={b.id}
                className="cursor-pointer border-b border-surface-200 hover:bg-surface-100"
                onClick={() => navigate("/" + ROUTES.BRAND_DETAIL(b.id))}
              >
                <td className="py-3 font-medium text-gray-900">{b.name}</td>
                <td className="py-3 text-gray-600">{b.managerCount}</td>
                <td className="py-3 text-gray-500">
                  {new Date(b.createdAt).toLocaleDateString("ru")}
                </td>
                <td className="py-3 text-right">
                  {deleteConfirm === b.id ? (
                    <span className="space-x-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          deleteMut.mutate(b.id);
                        }}
                        disabled={deleteMut.isPending}
                        className="text-red-600 font-medium hover:text-red-800 disabled:opacity-50"
                      >
                        {deleteMut.isPending ? "Удаление..." : "Да, удалить"}
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setDeleteConfirm(null);
                        }}
                        className="text-gray-500 hover:text-gray-700"
                      >
                        Отмена
                      </button>
                    </span>
                  ) : (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setDeleteConfirm(b.id);
                      }}
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

      {error && !showCreate && <p className="mt-4 text-sm text-red-600" role="alert">{error}</p>}
    </div>
  );
}
