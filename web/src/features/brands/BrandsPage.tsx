import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { listBrands, createBrand, deleteBrand } from "@/api/brands";

export default function BrandsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [error, setError] = useState("");

  const { data, isLoading } = useQuery({
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
    onError: () => setError("Не удалось создать бренд"),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteBrand(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["brands"] });
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
          {error && <p className="text-sm text-red-600">{error}</p>}
        </form>
      )}

      {isLoading ? (
        <p className="mt-6 text-gray-500">Загрузка...</p>
      ) : brands.length === 0 ? (
        <p className="mt-6 text-gray-500">Нет брендов</p>
      ) : (
        <table className="mt-6 w-full text-left text-sm">
          <thead>
            <tr className="border-b border-surface-300 text-gray-500">
              <th className="pb-2 font-medium">Название</th>
              <th className="pb-2 font-medium">Менеджеры</th>
              <th className="pb-2 font-medium">Создан</th>
              <th className="pb-2 font-medium" />
            </tr>
          </thead>
          <tbody>
            {brands.map((b) => (
              <tr
                key={b.id}
                className="cursor-pointer border-b border-surface-200 hover:bg-surface-100"
                onClick={() => navigate(`/brands/${b.id}`)}
              >
                <td className="py-3 font-medium text-gray-900">{b.name}</td>
                <td className="py-3 text-gray-600">{b.managerCount}</td>
                <td className="py-3 text-gray-500">
                  {new Date(b.createdAt).toLocaleDateString("ru")}
                </td>
                <td className="py-3 text-right">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      if (confirm("Удалить бренд?")) deleteMut.mutate(b.id);
                    }}
                    className="text-red-500 hover:text-red-700"
                  >
                    Удалить
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
