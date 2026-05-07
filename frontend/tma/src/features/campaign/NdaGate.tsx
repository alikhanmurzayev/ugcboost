import { useState } from "react";

export function NdaGate({ onAccept }: { onAccept: () => void }) {
  const [checked, setChecked] = useState(false);

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="nda-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4 py-6"
    >
      <div className="w-full max-w-sm rounded-card bg-white p-6 shadow-2xl">
        <h2
          id="nda-title"
          className="text-lg font-bold text-gray-900"
        >
          Конфиденциальность брифа
        </h2>
        <p className="mt-3 text-sm leading-relaxed text-gray-700">
          Открывая бриф рекламной кампании, вы получаете коммерческую
          информацию платформы UGC boost. В соответствии с Пользовательским
          соглашением и Политикой обработки персональных данных вы обязуетесь
          не разглашать содержимое брифа третьим лицам и не использовать его
          вне условий коллаборации. Нарушение влечет ответственность согласно
          действующему законодательству Республики Казахстан.
        </p>
        <label
          htmlFor="nda-checkbox"
          className="mt-5 flex cursor-pointer select-none items-start gap-3"
        >
          <input
            id="nda-checkbox"
            type="checkbox"
            checked={checked}
            onChange={(e) => setChecked(e.target.checked)}
            className="mt-0.5 h-5 w-5 flex-shrink-0 rounded accent-primary"
            data-testid="nda-checkbox"
          />
          <span className="text-sm leading-snug text-gray-900">
            Я согласен с условиями неразглашения
          </span>
        </label>
        <button
          type="button"
          onClick={onAccept}
          disabled={!checked}
          className={
            "mt-6 w-full rounded-button py-3 text-base font-semibold transition-colors " +
            (checked
              ? "bg-primary text-white hover:bg-primary-600 active:bg-primary-700"
              : "cursor-not-allowed bg-surface-300 text-gray-400")
          }
          data-testid="nda-accept-button"
        >
          Посмотреть
        </button>
      </div>
    </div>
  );
}
