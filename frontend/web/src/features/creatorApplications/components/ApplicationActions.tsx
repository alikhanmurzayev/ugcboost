import { useTranslation } from "react-i18next";
import type { ApplicationDetail, ApplicationStatus } from "../types";
import { useDrawerContext } from "./drawerContext";
import RejectApplicationDialog from "./RejectApplicationDialog";

interface ApplicationActionsProps {
  application: ApplicationDetail | undefined;
}

export default function ApplicationActions({ application }: ApplicationActionsProps) {
  const { onApiError, onCloseDrawer } = useDrawerContext();
  const { t } = useTranslation("creatorApplications");

  if (!application) return null;

  const status: ApplicationStatus = application.status;
  switch (status) {
    case "verification":
      return (
        <div className="flex items-center justify-end" data-testid="application-actions">
          <RejectApplicationDialog
            applicationId={application.id}
            hasTelegram={!!application.telegramLink}
            onApiError={onApiError}
            onCloseDrawer={onCloseDrawer}
          />
        </div>
      );
    case "moderation":
      return (
        <div
          className="flex items-center justify-end gap-3"
          data-testid="application-actions"
        >
          <RejectApplicationDialog
            applicationId={application.id}
            hasTelegram={!!application.telegramLink}
            onApiError={onApiError}
            onCloseDrawer={onCloseDrawer}
          />
          <button
            type="button"
            disabled
            data-testid="approve-button"
            title={t("actions.approveDisabledHint")}
            className="cursor-not-allowed rounded-button border border-emerald-600 bg-white px-4 py-2 text-sm font-semibold text-emerald-700 opacity-60"
          >
            {t("actions.approve")}
          </button>
        </div>
      );
    case "awaiting_contract":
    case "contract_sent":
    case "signed":
    case "rejected":
    case "withdrawn":
      return null;
    default: {
      const _exhaustive: never = status;
      return _exhaustive;
    }
  }
}
