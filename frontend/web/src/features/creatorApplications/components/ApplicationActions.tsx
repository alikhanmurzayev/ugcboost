import type { ApplicationDetail, ApplicationStatus } from "../types";
import { useDrawerContext } from "./drawerContext";
import RejectApplicationDialog from "./RejectApplicationDialog";

interface ApplicationActionsProps {
  application: ApplicationDetail | undefined;
}

export default function ApplicationActions({ application }: ApplicationActionsProps) {
  const { onApiError, onCloseDrawer } = useDrawerContext();

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
