import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getBrand } from "@/api/brands";
import { ROUTES } from "@/shared/constants/routes";
import { brandKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import BrandEditForm from "./components/BrandEditForm";
import ManagerList from "./components/ManagerList";
import AssignManagerForm from "./components/AssignManagerForm";

export default function BrandDetailPage() {
  const { t } = useTranslation("brands");
  const { brandId } = useParams<{ brandId: string }>();
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: brandKeys.detail(brandId as string),
    queryFn: () => getBrand(brandId as string),
    enabled: !!brandId,
  });

  if (!brandId) return <ErrorState message={t("notFound")} />;
  if (isLoading) return <Spinner className="mt-12" />;
  if (isError || !data) return <ErrorState message={t("loadDetailError")} onRetry={() => void refetch()} />;

  const brand = data.data;

  return (
    <div className="max-w-2xl">
      <button
        onClick={() => navigate("/" + ROUTES.BRANDS)}
        className="mb-4 text-sm text-primary hover:underline"
      >
        &larr; {t("backToList")}
      </button>

      <div className="rounded-card border border-surface-300 bg-white p-6">
        {editing ? (
          <BrandEditForm brandId={brandId} currentName={brand.name} onClose={() => setEditing(false)} />
        ) : (
          <div className="flex items-center justify-between">
            <h1 className="text-2xl font-bold text-gray-900">{brand.name}</h1>
            <button
              onClick={() => setEditing(true)}
              className="rounded-button border border-surface-300 px-3 py-1.5 text-sm text-gray-600 hover:bg-surface-200"
              data-testid="edit-brand-button"
            >
              {t("common:edit")}
            </button>
          </div>
        )}
        <p className="mt-2 text-sm text-gray-500">
          {t("created", { date: new Date(brand.createdAt).toLocaleDateString("ru") })}
        </p>
      </div>

      <div className="mt-6 rounded-card border border-surface-300 bg-white p-6">
        <h2 className="text-lg font-bold text-gray-900">{t("managers")}</h2>
        <ManagerList brandId={brandId} managers={brand.managers} />
        <AssignManagerForm brandId={brandId} />
      </div>
    </div>
  );
}
