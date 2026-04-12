import { useAuthStore } from "@/stores/auth";

export default function DashboardPage() {
  const user = useAuthStore((s) => s.user);

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Дашборд</h1>
      <p className="mt-2 text-gray-500">
        Добро пожаловать, {user?.email}
      </p>
    </div>
  );
}
