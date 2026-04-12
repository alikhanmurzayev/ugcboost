import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listBrands } from "@/api/brands";
import { brandKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import CreateBrandForm from "./components/CreateBrandForm";
import BrandList from "./components/BrandList";

export default function BrandsPage() {
  const [showCreate, setShowCreate] = useState(false);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: brandKeys.all(),
    queryFn: () => listBrands(),
  });

  const brands = data?.data.brands ?? [];

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Бренды</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-button bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary/90"
          data-testid="create-brand-button"
        >
          Создать бренд
        </button>
      </div>

      {showCreate && <CreateBrandForm onClose={() => setShowCreate(false)} />}

      {isLoading ? (
        <Spinner className="mt-6" />
      ) : isError ? (
        <ErrorState message="Не удалось загрузить бренды" onRetry={() => void refetch()} />
      ) : (
        <BrandList brands={brands} />
      )}
    </div>
  );
}
