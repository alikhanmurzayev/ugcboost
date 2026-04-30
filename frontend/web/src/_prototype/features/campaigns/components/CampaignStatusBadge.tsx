import { useTranslation } from "react-i18next";
import type { CampaignStatus } from "../types";

const VARIANTS: Record<CampaignStatus, string> = {
  draft: "bg-surface-200 text-gray-700",
  pending_moderation: "bg-amber-100 text-amber-800",
  rejected: "bg-red-100 text-red-800",
  active: "bg-emerald-100 text-emerald-800",
  completed: "bg-sky-100 text-sky-800",
};

export default function CampaignStatusBadge({
  status,
}: {
  status: CampaignStatus;
}) {
  const { t } = useTranslation("prototype_campaigns");
  return (
    <span
      className={`inline-flex whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${VARIANTS[status]}`}
      data-testid={`campaign-status-${status}`}
    >
      {t(`statuses.${status}`)}
    </span>
  );
}
