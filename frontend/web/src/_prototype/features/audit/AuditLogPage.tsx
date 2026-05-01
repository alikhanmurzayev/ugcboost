// Prototype audit log — static mock so the admin sidebar's /audit link lands
// somewhere visually consistent without hitting the real audit API.
const MOCK_ENTRIES = [
  {
    id: "1",
    actor: "admin@ugcboost.kz",
    action: "creator_application.approve",
    target: "applicaton:abc-123",
    at: "2026-04-30 14:22",
  },
  {
    id: "2",
    actor: "admin@ugcboost.kz",
    action: "brand.create",
    target: "brand:demo-brand",
    at: "2026-04-30 12:08",
  },
  {
    id: "3",
    actor: "admin@ugcboost.kz",
    action: "manager.assign",
    target: "brand:demo-brand",
    at: "2026-04-30 12:09",
  },
];

export default function AuditLogPage() {
  return (
    <div data-testid="prototype-audit-page">
      <h1 className="text-2xl font-bold text-gray-900">Журнал аудита</h1>
      <p className="mt-2 text-sm text-gray-500">
        Действия пользователей и системных событий.
      </p>

      <div className="mt-6 overflow-hidden rounded-card border border-surface-300 bg-white">
        <table className="w-full text-sm">
          <thead className="bg-surface-100 text-left text-xs uppercase tracking-wide text-gray-500">
            <tr>
              <th className="px-4 py-2">Время</th>
              <th className="px-4 py-2">Кто</th>
              <th className="px-4 py-2">Действие</th>
              <th className="px-4 py-2">Объект</th>
            </tr>
          </thead>
          <tbody>
            {MOCK_ENTRIES.map((entry) => (
              <tr key={entry.id} className="border-t border-surface-200">
                <td className="px-4 py-2 text-gray-700">{entry.at}</td>
                <td className="px-4 py-2 text-gray-700">{entry.actor}</td>
                <td className="px-4 py-2 font-mono text-xs text-gray-700">{entry.action}</td>
                <td className="px-4 py-2 font-mono text-xs text-gray-500">{entry.target}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
