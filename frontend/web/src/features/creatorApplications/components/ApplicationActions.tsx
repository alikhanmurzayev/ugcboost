import type { ApplicationDetail, ApplicationStatus } from "../types";
import { useDrawerContext } from "./drawerContext";
import RejectApplicationDialog from "./RejectApplicationDialog";
import ApproveApplicationDialog from "./ApproveApplicationDialog";

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
          <ApproveApplicationDialog
            applicationId={application.id}
            onApiError={onApiError}
            onCloseDrawer={onCloseDrawer}
          />
        </div>
      );
    case "approved":
    case "rejected":
    case "withdrawn":
      return null;
    default: {
      const _exhaustive: never = status;
      return _exhaustive;
    }
  }
}
