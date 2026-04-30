import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Campaign, CampaignApplication } from "../types";
import { calcBreakdown } from "./CostSummary";

interface Props {
  campaign: Campaign;
  approved: CampaignApplication[];
  brandCity: string;
  brandName: string;
  onClose: () => void;
  onSubmit: (data: ContractData) => void;
}

export interface ContractRequisites {
  legalName: string;
  bin: string;
  bank: string;
  account: string;
  address: string;
}

export interface ContractData {
  requisites: ContractRequisites;
}

const STEPS = ["brief", "creators", "cost", "requisites"] as const;
type Step = (typeof STEPS)[number];

const EMPTY_REQUISITES: ContractRequisites = {
  legalName: "",
  bin: "",
  bank: "",
  account: "",
  address: "",
};

export default function ContractWizard({
  campaign,
  approved,
  brandCity,
  brandName,
  onClose,
  onSubmit,
}: Props) {
  const { t } = useTranslation("prototype_campaigns");
  const [stepIdx, setStepIdx] = useState(0);
  const [confirmed, setConfirmed] = useState<Record<Step, boolean>>({
    brief: false,
    creators: false,
    cost: false,
    requisites: false,
  });
  const [requisites, setRequisites] =
    useState<ContractRequisites>(EMPTY_REQUISITES);
  const [submitting, setSubmitting] = useState(false);

  const currentStep = STEPS[stepIdx]!;
  const isLast = stepIdx === STEPS.length - 1;
  const isFirst = stepIdx === 0;
  const canProceed = confirmed[currentStep] && (currentStep !== "requisites" || requisitesValid(requisites));

  useEffect(() => {
    function handle(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [onClose]);

  function next() {
    if (!canProceed) return;
    if (isLast) {
      setSubmitting(true);
      onSubmit({ requisites });
      return;
    }
    setStepIdx((i) => i + 1);
  }
  function prev() {
    if (isFirst) return;
    setStepIdx((i) => i - 1);
  }

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-gray-900/60 p-4"
      onClick={onClose}
      data-testid="contract-wizard"
    >
      <div
        className="flex max-h-[92vh] w-full max-w-3xl flex-col rounded-2xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-3 border-b border-surface-200 px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              {t("wizardContract.title")}
            </h2>
            <p className="mt-0.5 text-xs text-gray-500">
              {t("wizardContract.stepOf", {
                current: stepIdx + 1,
                total: STEPS.length,
                name: t(`wizardContract.steps.${currentStep}`),
              })}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("drawerClose")}
            className="rounded-full p-1 text-gray-500 hover:bg-surface-100"
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </header>

        <div className="flex border-b border-surface-200 px-6 py-3">
          {STEPS.map((s, i) => (
            <div
              key={s}
              className={`flex flex-1 items-center gap-2 text-xs ${
                i === stepIdx
                  ? "font-medium text-primary"
                  : confirmed[s]
                    ? "text-emerald-600"
                    : "text-gray-400"
              }`}
            >
              <span
                className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] ${
                  i === stepIdx
                    ? "bg-primary text-white"
                    : confirmed[s]
                      ? "bg-emerald-100 text-emerald-700"
                      : "bg-surface-200 text-gray-500"
                }`}
              >
                {confirmed[s] && i !== stepIdx ? "✓" : i + 1}
              </span>
              <span className="truncate">
                {t(`wizardContract.steps.${s}`)}
              </span>
            </div>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-5">
          {currentStep === "brief" && (
            <BriefStep campaign={campaign} brandName={brandName} />
          )}
          {currentStep === "creators" && (
            <CreatorsStep approved={approved} />
          )}
          {currentStep === "cost" && (
            <CostStep
              campaign={campaign}
              approved={approved}
              brandCity={brandCity}
            />
          )}
          {currentStep === "requisites" && (
            <RequisitesStep
              value={requisites}
              onChange={setRequisites}
              brandName={brandName}
            />
          )}
        </div>

        <footer className="flex items-center justify-between gap-3 border-t border-surface-200 bg-surface-50 px-6 py-4">
          <label className="flex cursor-pointer items-center gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={confirmed[currentStep]}
              onChange={(e) =>
                setConfirmed({ ...confirmed, [currentStep]: e.target.checked })
              }
              className="h-4 w-4 rounded border-gray-300 accent-primary text-primary focus:ring-primary"
              data-testid="wizard-confirm"
            />
            <span>{t("wizardContract.confirm")}</span>
          </label>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={prev}
              disabled={isFirst || submitting}
              className="rounded-button border border-surface-300 bg-white px-4 py-2 text-sm font-semibold text-gray-700 hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-40"
              data-testid="wizard-prev"
            >
              ← {t("wizardContract.back")}
            </button>
            <button
              type="button"
              onClick={next}
              disabled={!canProceed || submitting}
              className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-40"
              data-testid="wizard-next"
            >
              {isLast ? t("wizardContract.sign") : t("wizardContract.next")}
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}

function BriefStep({
  campaign,
  brandName,
}: {
  campaign: Campaign;
  brandName: string;
}) {
  const { t } = useTranslation("prototype_campaigns");
  return (
    <div className="space-y-3 text-sm">
      <p className="text-xs text-gray-500">{t("wizardContract.briefIntro")}</p>
      <div className="rounded-card border border-surface-200 bg-surface-50 p-4">
        <p className="text-xs uppercase tracking-wide text-gray-500">
          {brandName} · {t(`types.${campaign.type}`)}
        </p>
        <p className="mt-1 text-base font-semibold text-gray-900">
          {campaign.title}
        </p>
        <p className="mt-2 whitespace-pre-wrap text-sm text-gray-700">
          {campaign.description}
        </p>
      </div>
    </div>
  );
}

function CreatorsStep({ approved }: { approved: CampaignApplication[] }) {
  const { t } = useTranslation("prototype_campaigns");
  return (
    <div className="space-y-3 text-sm">
      <p className="text-xs text-gray-500">
        {t("wizardContract.creatorsIntro", { count: approved.length })}
      </p>
      <div className="overflow-hidden rounded-card border border-surface-200">
        <table className="w-full text-left text-sm">
          <thead className="bg-surface-50 text-xs uppercase tracking-wide text-gray-500">
            <tr>
              <th className="px-3 py-2">№</th>
              <th className="px-3 py-2">{t("columns.fullName")}</th>
              <th className="px-3 py-2">{t("columns.city")}</th>
              <th className="px-3 py-2 text-right">
                {t("columns.followers")}
              </th>
            </tr>
          </thead>
          <tbody>
            {approved.map((a, i) => (
              <tr key={a.id} className="border-t border-surface-200">
                <td className="px-3 py-2 text-gray-400 tabular-nums">
                  {i + 1}
                </td>
                <td className="px-3 py-2 font-medium text-gray-900">
                  {a.creator.lastName} {a.creator.firstName}
                </td>
                <td className="px-3 py-2 text-gray-700">
                  {a.creator.city.name}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">
                  {a.creator.metrics.followers.toLocaleString("ru")}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function CostStep({
  campaign,
  approved,
  brandCity,
}: {
  campaign: Campaign;
  approved: CampaignApplication[];
  brandCity: string;
}) {
  const { t } = useTranslation("prototype_campaigns");
  const b = calcBreakdown(campaign, approved, brandCity);
  return (
    <div className="space-y-3 text-sm">
      <p className="text-xs text-gray-500">{t("wizardContract.costIntro")}</p>
      <ul className="space-y-2 rounded-card border border-surface-200 bg-surface-50 p-4">
        {b.hasPayment && (
          <li className="flex justify-between">
            <span>
              {t("cost.creators")}: {b.creators} × {money(campaign.paymentAmount ?? 0)}
            </span>
            <span className="tabular-nums">{money(b.paymentBase)}</span>
          </li>
        )}
        {b.hasLogistics && (
          <li className="flex justify-between">
            <span>{t("cost.logistics")}</span>
            <span className="tabular-nums">{money(b.logistics)}</span>
          </li>
        )}
        <li className="flex justify-between border-t border-surface-200 pt-2">
          <span>{t("cost.subtotal")}</span>
          <span className="tabular-nums">{money(b.subtotal)}</span>
        </li>
        <li className="flex justify-between">
          <span>{t("cost.vat")} 16%</span>
          <span className="tabular-nums">{money(b.vat)}</span>
        </li>
        <li className="flex items-baseline justify-between border-t border-surface-200 pt-2">
          <span className="text-base font-semibold">{t("cost.total")}</span>
          <span className="text-xl font-bold tabular-nums text-gray-900">
            {money(b.total)}
          </span>
        </li>
      </ul>
      {b.hasBarter && (
        <p className="text-xs text-gray-500">{t("cost.barterNote")}</p>
      )}
    </div>
  );
}

function RequisitesStep({
  value,
  onChange,
  brandName,
}: {
  value: ContractRequisites;
  onChange: (v: ContractRequisites) => void;
  brandName: string;
}) {
  const { t } = useTranslation("prototype_campaigns");
  function patch(p: Partial<ContractRequisites>) {
    onChange({ ...value, ...p });
  }
  return (
    <div className="space-y-3 text-sm">
      <p className="text-xs text-gray-500">
        {t("wizardContract.requisitesIntro", { brand: brandName })}
      </p>
      <Field label={t("wizardContract.legalName")} required>
        <input
          type="text"
          value={value.legalName}
          onChange={(e) => patch({ legalName: e.target.value })}
          placeholder={`ТОО «${brandName}»`}
          className={inputCls}
          data-testid="requisites-legal-name"
        />
      </Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("wizardContract.bin")} required>
          <input
            type="text"
            inputMode="numeric"
            value={value.bin}
            onChange={(e) =>
              patch({ bin: e.target.value.replace(/\D/g, "").slice(0, 12) })
            }
            placeholder="123456789012"
            className={inputCls}
            data-testid="requisites-bin"
          />
        </Field>
        <Field label={t("wizardContract.bank")} required>
          <input
            type="text"
            value={value.bank}
            onChange={(e) => patch({ bank: e.target.value })}
            placeholder="АО «Kaspi Bank»"
            className={inputCls}
            data-testid="requisites-bank"
          />
        </Field>
      </div>
      <Field label={t("wizardContract.account")} required>
        <input
          type="text"
          value={value.account}
          onChange={(e) => patch({ account: e.target.value })}
          placeholder="KZ123456789012345678"
          className={inputCls}
          data-testid="requisites-account"
        />
      </Field>
      <Field label={t("wizardContract.address")} required>
        <input
          type="text"
          value={value.address}
          onChange={(e) => patch({ address: e.target.value })}
          placeholder="Алматы, ул. ..."
          className={inputCls}
          data-testid="requisites-address"
        />
      </Field>
    </div>
  );
}

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700">
        {label}
        {required && <span className="ml-1 text-red-500">*</span>}
      </label>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}

function requisitesValid(r: ContractRequisites): boolean {
  return (
    r.legalName.trim().length > 0 &&
    r.bin.length === 12 &&
    r.bank.trim().length > 0 &&
    r.account.trim().length > 0 &&
    r.address.trim().length > 0
  );
}

function money(n: number): string {
  return `${n.toLocaleString("ru")} ₸`;
}

const inputCls =
  "w-full rounded-button border border-gray-300 bg-white px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100";
