// Prototype dashboard — purely visual, no backend calls. Mirrors the main
// DashboardPage shape (header + welcome) so navigating to it from the
// prototype sidebar lands on the same kind of placeholder Aidana saw.
export default function DashboardPage() {
  return (
    <div data-testid="prototype-dashboard-page">
      <h1 className="text-2xl font-bold text-gray-900">Дашборд</h1>
      <p className="mt-2 text-gray-500">
        Добро пожаловать в UGC boost (прототип)
      </p>
    </div>
  );
}
