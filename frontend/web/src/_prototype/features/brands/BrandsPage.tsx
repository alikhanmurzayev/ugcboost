// Prototype "Мои бренды" — static mock so /prototype shows what Aidana
// designed (a single demo brand) without hitting the real /v1/brands API.
import { Link } from "react-router-dom";

const MOCK_BRANDS = [
  {
    id: "demo-brand",
    name: "Demo Brand",
    description: "Демо-бренд прототипа. Используется только для UI-демонстрации.",
  },
];

export default function BrandsPage() {
  return (
    <div data-testid="prototype-brands-page">
      <h1 className="text-2xl font-bold text-gray-900">Мои бренды</h1>
      <p className="mt-2 text-sm text-gray-500">
        Список брендов, которыми вы управляете.
      </p>

      <ul className="mt-6 space-y-3">
        {MOCK_BRANDS.map((brand) => (
          <li
            key={brand.id}
            className="rounded-card border border-surface-300 bg-white p-4 transition hover:border-primary-200 hover:shadow-sm"
          >
            <Link
              to={`/prototype/brands/${brand.id}`}
              className="block"
              data-testid={`prototype-brand-card-${brand.id}`}
            >
              <h2 className="text-lg font-semibold text-gray-900">{brand.name}</h2>
              <p className="mt-1 text-sm text-gray-500">{brand.description}</p>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
