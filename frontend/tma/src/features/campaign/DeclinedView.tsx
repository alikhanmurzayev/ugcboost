type DeclinedViewProps = {
  alreadyDecided?: boolean;
};

export function DeclinedView({ alreadyDecided = false }: DeclinedViewProps) {
  return (
    <div
      data-testid="tma-declined-view"
      className="flex min-h-screen items-center justify-center bg-surface px-6"
    >
      <div className="max-w-sm text-center">
        <h1 className="text-2xl font-bold text-gray-900">
          Вы отказались от участия в EURASIAN FASHION WEEK
        </h1>
        {alreadyDecided && (
          <p
            data-testid="tma-already-decided-banner"
            className="mx-auto mt-4 max-w-xs rounded-md bg-surface-100 px-4 py-2 text-sm text-gray-700"
          >
            Вы уже отказывались от участия в этой кампании.
          </p>
        )}
        <p className="mt-4 text-base leading-relaxed text-gray-600">
          Спасибо за уделенное время. Оставайтесь с нами на платформе — в
          будущем появятся другие масштабные проекты.
        </p>
      </div>
    </div>
  );
}
