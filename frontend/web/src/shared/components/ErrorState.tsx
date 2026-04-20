import { useTranslation } from "react-i18next";

interface ErrorStateProps {
  message?: string;
  onRetry?: () => void;
}

export default function ErrorState({ message, onRetry }: ErrorStateProps) {
  const { t } = useTranslation("common");

  return (
    <div role="alert" data-testid="error-state" className="flex flex-col items-center justify-center py-12 text-center">
      <p className="text-sm text-red-600">{message ?? t("error")}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-3 rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
          data-testid="error-retry-button"
        >
          {t("retry")}
        </button>
      )}
    </div>
  );
}
