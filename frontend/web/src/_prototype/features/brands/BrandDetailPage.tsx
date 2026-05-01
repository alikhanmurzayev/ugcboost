// Prototype brand detail — static mock, no backend calls.
import { Link, useParams } from "react-router-dom";

export default function BrandDetailPage() {
  const { brandId } = useParams<{ brandId: string }>();

  return (
    <div data-testid="prototype-brand-detail-page">
      <Link
        to="/prototype/brands"
        className="text-sm text-gray-500 transition hover:text-gray-700"
      >
        ← К списку брендов
      </Link>

      <h1 className="mt-4 text-2xl font-bold text-gray-900">Demo Brand</h1>
      <p className="mt-1 text-xs text-gray-400">id: {brandId}</p>

      <div className="mt-6 rounded-card border border-surface-300 bg-white p-4">
        <p className="text-sm text-gray-700">
          Это демо-карточка бренда в прототипе. В рабочем приложении здесь
          будет управление менеджерами и логотипом бренда.
        </p>
      </div>
    </div>
  );
}
