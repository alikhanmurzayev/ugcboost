interface ErrorStateProps {
  message?: string;
  onRetry?: () => void;
}

export default function ErrorState({
  message = "Произошла ошибка",
  onRetry,
}: ErrorStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-12 text-center">
      <p className="text-sm text-red-600">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-3 rounded-button border border-surface-300 px-4 py-2 text-sm text-gray-600 hover:bg-surface-200"
        >
          Повторить
        </button>
      )}
    </div>
  );
}
