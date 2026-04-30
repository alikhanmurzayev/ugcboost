import { useTranslation } from "react-i18next";
import type { Campaign, CampaignApplication } from "../types";

const VAT_RATE = 0.16;
const LOGISTICS_SAME_CITY = 1500;
const LOGISTICS_OTHER_CITY = 2500;

interface Props {
  campaign: Campaign;
  approved: CampaignApplication[];
  brandCity: string;
}

interface Breakdown {
  creators: number;
  paymentBase: number;
  logistics: number;
  subtotal: number;
  vat: number;
  total: number;
  hasLogistics: boolean;
  hasBarter: boolean;
  hasPayment: boolean;
}

export function calcBreakdown(
  campaign: Campaign,
  approved: CampaignApplication[],
  brandCity: string,
): Breakdown {
  const creators = approved.length;
  const hasPayment =
    campaign.paymentType === "fixed" ||
    campaign.paymentType === "barter_fixed";
  const hasBarter =
    campaign.paymentType === "barter" ||
    campaign.paymentType === "barter_fixed";
  // Logistics is only billed when the brand actually ships goods to creators —
  // i.e. physical product / food bartered to a person (not a visit, not an event).
  const hasLogistics =
    hasBarter &&
    !campaign.requiresVisit &&
    (campaign.type === "product" || campaign.type === "food");

  const paymentBase = hasPayment
    ? (campaign.paymentAmount ?? 0) * creators
    : 0;

  let logistics = 0;
  if (hasLogistics) {
    for (const a of approved) {
      logistics +=
        a.creator.city.code === brandCity
          ? LOGISTICS_SAME_CITY
          : LOGISTICS_OTHER_CITY;
    }
  }

  const subtotal = paymentBase + logistics;
  const vat = Math.round(subtotal * VAT_RATE);
  const total = subtotal + vat;

  return {
    creators,
    paymentBase,
    logistics,
    subtotal,
    vat,
    total,
    hasLogistics,
    hasBarter,
    hasPayment,
  };
}

export default function CostSummary({ campaign, approved, brandCity }: Props) {
  const { t } = useTranslation("prototype_campaigns");
  const b = calcBreakdown(campaign, approved, brandCity);

  return (
    <section
      className="rounded-2xl border border-surface-200 bg-white p-5 shadow-sm"
      data-testid="cost-summary"
    >
      <h3 className="mb-4 border-b border-surface-200 pb-3 text-base font-semibold text-gray-900">
        {t("cost.title")}
      </h3>

      <ul className="space-y-2 text-sm">
        {b.hasPayment && (
          <Row
            label={`${t("cost.creators")}, ${b.creators} × ${money(campaign.paymentAmount ?? 0)}`}
            value={money(b.paymentBase)}
          />
        )}
        {b.hasLogistics && (
          <Row
            label={`${t("cost.logistics")} (${LOGISTICS_SAME_CITY} ₸ — свой город / ${LOGISTICS_OTHER_CITY} ₸ — другой)`}
            value={money(b.logistics)}
          />
        )}
      </ul>

      <div className="mt-4 space-y-2 border-t border-surface-200 pt-3 text-sm">
        <Row label={t("cost.subtotal")} value={money(b.subtotal)} />
        <Row label={`${t("cost.vat")} (${Math.round(VAT_RATE * 100)}%)`} value={money(b.vat)} />
      </div>

      <div className="mt-4 flex items-baseline justify-between border-t border-surface-200 pt-3">
        <span className="text-base font-semibold text-gray-900">
          {t("cost.total")}
        </span>
        <span className="text-2xl font-bold tabular-nums text-gray-900">
          {money(b.total)}
        </span>
      </div>

      {b.hasBarter && (
        <p className="mt-3 text-xs text-gray-500">{t("cost.barterNote")}</p>
      )}
    </section>
  );
}

function Row({
  label,
  value,
  muted,
}: {
  label: string;
  value: React.ReactNode;
  muted?: boolean;
}) {
  return (
    <li className="flex items-baseline justify-between gap-3">
      <span className={muted ? "text-gray-500" : "text-gray-700"}>{label}</span>
      <span className={`shrink-0 tabular-nums ${muted ? "text-gray-500" : "text-gray-900"}`}>
        {value}
      </span>
    </li>
  );
}

function money(n: number): string {
  return `${n.toLocaleString("ru")} ₸`;
}
