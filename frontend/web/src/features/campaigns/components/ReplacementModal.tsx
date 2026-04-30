import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import SocialLink from "@/features/creatorApplications/components/SocialLink";
import { CategoryChips } from "@/features/creatorApplications/components/CategoryChip";
import type { CampaignApplication } from "../types";

interface Props {
  declined: CampaignApplication;
  candidates: CampaignApplication[];
  onClose: () => void;
  onPick: (replacementApplicationId: string) => void;
  isSubmitting: boolean;
}

export default function ReplacementModal({
  declined,
  candidates,
  onClose,
  onPick,
  isSubmitting,
}: Props) {
  const { t } = useTranslation("campaigns");
  const [chosenId, setChosenId] = useState<string | null>(null);

  useEffect(() => {
    function handle(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [onClose]);

  const c = declined.creator;

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-gray-900/60 p-4"
      onClick={onClose}
      data-testid="replacement-modal"
    >
      <div
        className="flex max-h-[88vh] w-full max-w-3xl flex-col rounded-2xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="border-b border-surface-200 px-6 py-4">
          <h2 className="text-lg font-semibold text-gray-900">
            {t("replacement.title", {
              name: `${c.lastName} ${c.firstName}`,
            })}
          </h2>
          <p className="mt-1 text-xs text-gray-500">
            {t("replacement.subtitle")}
          </p>
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {candidates.length === 0 ? (
            <p className="rounded-card border border-dashed border-surface-300 bg-surface-50 px-4 py-8 text-center text-sm text-gray-500">
              {t("replacement.noCandidates")}
            </p>
          ) : (
            <ul className="space-y-2">
              {candidates.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => setChosenId(a.id)}
                    className={`w-full rounded-card border px-4 py-3 text-left transition ${
                      chosenId === a.id
                        ? "border-primary bg-primary-50"
                        : "border-surface-200 bg-white hover:bg-surface-50"
                    }`}
                    data-testid={`replacement-candidate-${a.id}`}
                  >
                    <div className="flex items-baseline justify-between gap-3">
                      <span className="text-sm font-semibold text-gray-900">
                        {a.creator.lastName} {a.creator.firstName}
                      </span>
                      <span className="text-xs text-gray-500">
                        {a.creator.age} · {a.creator.city.name}
                      </span>
                    </div>
                    <div
                      className="mt-1 flex flex-wrap items-center gap-3 text-xs"
                      onClick={(e) => e.stopPropagation()}
                      onKeyDown={(e) => e.stopPropagation()}
                      role="presentation"
                    >
                      {a.creator.socials.map((s) => (
                        <SocialLink
                          key={`${s.platform}-${s.handle}`}
                          platform={s.platform}
                          handle={s.handle}
                          showHandle
                        />
                      ))}
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-600">
                      <span>
                        {t("metricsFollowers")}:{" "}
                        <b className="tabular-nums text-gray-900">
                          {a.creator.metrics.followers.toLocaleString("ru")}
                        </b>
                      </span>
                      <span>
                        {t("metricsAvgViews")}:{" "}
                        <b className="tabular-nums text-gray-900">
                          {a.creator.metrics.avgViews.toLocaleString("ru")}
                        </b>
                      </span>
                      <span>
                        {t("metricsEr")}:{" "}
                        <b className="tabular-nums text-gray-900">
                          {a.creator.metrics.er}%
                        </b>
                      </span>
                    </div>
                    <div className="mt-2">
                      <CategoryChips categories={a.creator.categories} />
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <footer className="flex items-center justify-end gap-2 border-t border-surface-200 bg-surface-50 px-6 py-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-button border border-surface-300 bg-white px-4 py-2 text-sm font-semibold text-gray-700 hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {t("replacement.cancel")}
          </button>
          <button
            type="button"
            onClick={() => chosenId && onPick(chosenId)}
            disabled={!chosenId || isSubmitting}
            className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-40"
            data-testid="replacement-confirm"
          >
            {isSubmitting
              ? t("replacement.submitting")
              : t("replacement.confirm")}
          </button>
        </footer>
      </div>
    </div>
  );
}
