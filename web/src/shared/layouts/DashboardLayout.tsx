import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/api/auth";

const adminNav = [
  { to: "/", label: "Дашборд" },
  { to: "/creators", label: "Креаторы" },
  { to: "/campaigns", label: "Кампании" },
  { to: "/moderation", label: "Модерация" },
];

const brandNav = [
  { to: "/", label: "Дашборд" },
  { to: "/campaigns", label: "Кампании" },
  { to: "/creators", label: "Каталог креаторов" },
];

export default function DashboardLayout() {
  const user = useAuthStore((s) => s.user);
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const navigate = useNavigate();

  const nav = user?.role === "admin" ? adminNav : brandNav;

  async function handleLogout() {
    try {
      await logout();
    } catch {
      // ignore — clear local state anyway
    }
    clearAuth();
    navigate("/login", { replace: true });
  }

  return (
    <div className="flex min-h-screen bg-surface-100">
      {/* Sidebar */}
      <aside className="flex w-60 flex-col border-r border-surface-300 bg-white">
        <div className="border-b border-surface-300 px-5 py-4">
          <span className="text-lg font-bold text-gray-900">UGCBoost</span>
        </div>

        <nav className="flex-1 px-3 py-4">
          <ul className="space-y-1">
            {nav.map((item) => (
              <li key={item.to}>
                <NavLink
                  to={item.to}
                  end={item.to === "/"}
                  className={({ isActive }) =>
                    `block rounded-button px-3 py-2 text-sm font-medium transition ${
                      isActive
                        ? "bg-primary-50 text-primary"
                        : "text-gray-600 hover:bg-surface-200 hover:text-gray-900"
                    }`
                  }
                >
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        <div className="border-t border-surface-300 px-3 py-4">
          <div className="mb-3 px-3">
            <p className="truncate text-sm font-medium text-gray-900">
              {user?.email}
            </p>
            <p className="text-xs text-gray-500">
              {user?.role === "admin" ? "Админ" : "Бренд-менеджер"}
            </p>
          </div>
          <button
            onClick={handleLogout}
            className="w-full rounded-button px-3 py-2 text-left text-sm text-gray-600 transition hover:bg-surface-200 hover:text-gray-900"
          >
            Выйти
          </button>
        </div>
      </aside>

      {/* Main */}
      <main className="flex-1 overflow-y-auto p-8">
        <Outlet />
      </main>
    </div>
  );
}
