import { useTranslation } from "react-i18next";
import type { QualityIndicator } from "../types";

const COLORS: Record<QualityIndicator, string> = {
  green: "bg-emerald-500",
  orange: "bg-amber-500",
  red: "bg-red-500",
};

interface Props {
  value?: QualityIndicator;
  size?: "sm" | "md";
  withLabel?: boolean;
}

export default function QualityIndicatorDot({
  value,
  size = "sm",
  withLabel = false,
}: Props) {
  const { t } = useTranslation("prototype_creatorApplications");

  if (!value) {
    if (!withLabel) return null;
    return <span className="text-xs text-gray-400">{t("qualityIndicator.none")}</span>;
  }

  const sizeClass = size === "sm" ? "h-2.5 w-2.5" : "h-3 w-3";
  const label = t(`qualityIndicator.${value}`);
  const tooltip = t("qualityIndicator.tooltip");

  return (
    <span
      className="inline-flex items-center gap-2"
      data-testid={`quality-${value}`}
      title={`${label} · ${tooltip}`}
    >
      <span
        className={`inline-block rounded-full ${sizeClass} ${COLORS[value]}`}
        aria-label={label}
      />
      {withLabel && <span className="text-sm text-gray-700">{label}</span>}
    </span>
  );
}
