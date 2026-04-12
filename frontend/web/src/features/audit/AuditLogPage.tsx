import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { listAuditLogs } from "@/api/audit";
import { auditKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";

const ACTION_KEYS = [
  "login",
  "password_reset",
  "brand_create",
  "brand_update",
  "brand_delete",
  "manager_assign",
  "manager_remove",
] as const;

export default function AuditLogPage() {
  const { t } = useTranslation(["audit", "common"]);
  const [entityType, setEntityType] = useState("");
  const [action, setAction] = useState("");
  const [page, setPage] = useState(1);
  const perPage = 20;

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: auditKeys.list({ entityType, action, page }),
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
      <h1 className="text-2xl font-bold text-gray-900">{t("audit:title")}</h1>

      <div className="mt-4 flex gap-4">
        <select
          value={entityType}
          onChange={(e) => {
            setEntityType(e.target.value);
            setPage(1);
          }}
          className="rounded-button border border-surface-300 px-3 py-2 text-sm"
          aria-label={t("audit:entityTypeLabel")}
          data-testid="entity-type-filter"
        >
          <option value="">{t("audit:allTypes")}</option>
          <option value="user">{t("audit:entityTypes.user")}</option>
          <option value="brand">{t("audit:entityTypes.brand")}</option>
        </select>

        <select
          value={action}
          onChange={(e) => {
            setAction(e.target.value);
            setPage(1);
          }}
          className="rounded-button border border-surface-300 px-3 py-2 text-sm"
          aria-label={t("audit:actionLabel")}
          data-testid="action-filter"
        >
          <option value="">{t("audit:allActions")}</option>
          {ACTION_KEYS.map((key) => (
            <option key={key} value={key}>
              {t(`audit:actions.${key}`)}
            </option>
          ))}
        </select>
      </div>

      {isLoading ? (
        <Spinner className="mt-6" />
      ) : isError ? (
        <ErrorState message={t("audit:loadError")} onRetry={() => void refetch()} />
      ) : logs.length === 0 ? (
        <p className="mt-6 text-gray-500">{t("audit:noRecords")}</p>
      ) : (
        <>
          <table className="mt-6 w-full text-left text-sm" data-testid="audit-table">
            <thead>
              <tr className="border-b border-surface-300 text-gray-500">
                <th className="pb-2 font-medium">{t("audit:date")}</th>
                <th className="pb-2 font-medium">{t("audit:action")}</th>
                <th className="pb-2 font-medium">{t("audit:type")}</th>
                <th className="pb-2 font-medium">{t("audit:role")}</th>
                <th className="pb-2 font-medium">{t("audit:ip")}</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} className="border-b border-surface-200">
                  <td className="py-2 text-gray-500">
                    {new Date(log.createdAt).toLocaleString("ru")}
                  </td>
                  <td className="py-2 font-medium text-gray-900">
                    {t(`audit:actions.${log.action}`, { defaultValue: log.action })}
                  </td>
                  <td className="py-2 text-gray-600">{log.entityType}</td>
                  <td className="py-2 text-gray-600">{log.actorRole}</td>
                  <td className="py-2 font-mono text-xs text-gray-400">{log.ipAddress}</td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 && (
            <div className="mt-4 flex items-center gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="rounded-button border border-surface-300 px-3 py-1 text-sm disabled:opacity-50"
                data-testid="audit-prev-page"
              >
                {t("common:prev")}
              </button>
              <span className="text-sm text-gray-500">
                {page} / {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className="rounded-button border border-surface-300 px-3 py-1 text-sm disabled:opacity-50"
                data-testid="audit-next-page"
              >
                {t("common:next")}
              </button>
              <span className="text-xs text-gray-400">
                {t("common:total", { count: total })}
              </span>
            </div>
          )}
        </>
      )}
    </div>
  );
}
