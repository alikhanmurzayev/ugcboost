import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { deleteBrand } from "@/api/brands";
import type { BrandListItem } from "@/api/brands";
import { ROUTES } from "@/shared/constants/routes";
import { brandKeys } from "@/shared/constants/queryKeys";
import { getErrorMessage } from "@/shared/i18n/errors";
import { ApiError } from "@/api/client";

interface BrandListProps {
  brands: BrandListItem[];
}

export default function BrandList({ brands }: BrandListProps) {
  const { t } = useTranslation(["brands", "common"]);
  const queryClient = useQueryClient();
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [error, setError] = useState("");

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteBrand(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: brandKeys.all() });
      setDeleteConfirm(null);
    },
    onError: (err) => {
      setError(err instanceof ApiError ? getErrorMessage(err.code) : t("brands:deleteError"));
      setDeleteConfirm(null);
    },
  });

  if (brands.length === 0) {
    return <p className="mt-6 text-gray-500">{t("brands:noBrands")}</p>;
  }

  return (
    <>
      <table className="mt-6 w-full text-left text-sm" data-testid="brands-table">
        <thead>
          <tr className="border-b border-surface-300 text-gray-500">
            <th scope="col" className="pb-2 font-medium">{t("brands:name")}</th>
            <th scope="col" className="pb-2 font-medium">{t("brands:managers")}</th>
            <th scope="col" className="pb-2 font-medium">{t("brands:createdAt")}</th>
            <th scope="col" className="pb-2 font-medium" />
          </tr>
        </thead>
        <tbody>
          {brands.map((b) => (
            <tr
              key={b.id}
              className="border-b border-surface-200 hover:bg-surface-100"
              data-testid={`brand-row-${b.id}`}
            >
              <td className="py-3 font-medium text-gray-900">
                <Link to={"/" + ROUTES.BRAND_DETAIL(b.id)} className="text-primary hover:underline">
                  {b.name}
                </Link>
              </td>
              <td className="py-3 text-gray-600">{b.managerCount}</td>
              <td className="py-3 text-gray-500">
                {new Date(b.createdAt).toLocaleDateString("ru")}
              </td>
              <td className="py-3 text-right">
                {deleteConfirm === b.id ? (
                  <span className="space-x-2">
                    <button
                      onClick={() => deleteMut.mutate(b.id)}
                      disabled={deleteMut.isPending}
                      className="text-red-600 font-medium hover:text-red-800 disabled:opacity-50"
                    >
                      {deleteMut.isPending ? t("common:deleting") : t("common:confirmDelete")}
                    </button>
                    <button onClick={() => setDeleteConfirm(null)} className="text-gray-500 hover:text-gray-700">
                      {t("common:cancel")}
                    </button>
                  </span>
                ) : (
                  <button onClick={() => setDeleteConfirm(b.id)} className="text-red-500 hover:text-red-700">
                    {t("common:delete")}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {error && <p className="mt-4 text-sm text-red-600" role="alert">{error}</p>}
    </>
  );
}
