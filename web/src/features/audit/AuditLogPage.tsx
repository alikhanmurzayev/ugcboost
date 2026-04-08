import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listAuditLogs } from "@/api/audit";

const ACTION_LABELS: Record<string, string> = {
  login: "Вход",
  password_reset: "Сброс пароля",
  brand_create: "Создание бренда",
  brand_update: "Обновление бренда",
  brand_delete: "Удаление бренда",
  manager_assign: "Назначение менеджера",
  manager_remove: "Удаление менеджера",
};

export default function AuditLogPage() {
  const [entityType, setEntityType] = useState("");
  const [action, setAction] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 20;

  const { data, isLoading } = useQuery({
    queryKey: ["audit-logs", entityType, action, page],
    queryFn: () =>
      listAuditLogs({
        entity_type: entityType || undefined,
        action: action || undefined,
        page,
        per_page: perPage,
      }),
  });

  const logs = data?.data.logs ?? [];
  const total = data?.data.total ?? 0;
  const totalPages = Math.ceil(total / perPage);

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Журнал действий</h1>

      {/* Filters */}
      <div className="mt-4 flex gap-4">
        <select
          value={entityType}
          onChange={(e) => {
            setEntityType(e.target.value);
            setPage(1);
          }}
          className="rounded-button border border-surface-300 px-3 py-2 text-sm"
        >
          <option value="">Все типы</option>
          <option value="user">Пользователь</option>
          <option value="brand">Бренд</option>
        </select>

        <select
          value={action}
          onChange={(e) => {
            setAction(e.target.value);
            setPage(1);
          }}
          className="rounded-button border border-surface-300 px-3 py-2 text-sm"
        >
          <option value="">Все действия</option>
          {Object.entries(ACTION_LABELS).map(([key, label]) => (
            <option key={key} value={key}>
              {label}
            </option>
          ))}
        </select>
      </div>

      {/* Table */}
      {isLoading ? (
        <p className="mt-6 text-gray-500">Загрузка...</p>
      ) : logs.length === 0 ? (
        <p className="mt-6 text-gray-500">Нет записей</p>
      ) : (
        <>
          <table className="mt-6 w-full text-left text-sm">
            <thead>
              <tr className="border-b border-surface-300 text-gray-500">
                <th className="pb-2 font-medium">Дата</th>
                <th className="pb-2 font-medium">Действие</th>
                <th className="pb-2 font-medium">Тип</th>
                <th className="pb-2 font-medium">Роль</th>
                <th className="pb-2 font-medium">IP</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr
                  key={log.id}
                  className="border-b border-surface-200"
                >
                  <td className="py-2 text-gray-500">
                    {new Date(log.createdAt).toLocaleString("ru")}
                  </td>
                  <td className="py-2 font-medium text-gray-900">
                    {ACTION_LABELS[log.action] ?? log.action}
                  </td>
                  <td className="py-2 text-gray-600">{log.entityType}</td>
                  <td className="py-2 text-gray-600">{log.actorRole}</td>
                  <td className="py-2 font-mono text-xs text-gray-400">
                    {log.ipAddress}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="mt-4 flex items-center gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="rounded-button border border-surface-300 px-3 py-1 text-sm disabled:opacity-50"
              >
                Назад
              </button>
              <span className="text-sm text-gray-500">
                {page} / {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className="rounded-button border border-surface-300 px-3 py-1 text-sm disabled:opacity-50"
              >
                Вперёд
              </button>
              <span className="text-xs text-gray-400">
                Всего: {total}
              </span>
            </div>
          )}
        </>
      )}
    </div>
  );
}
