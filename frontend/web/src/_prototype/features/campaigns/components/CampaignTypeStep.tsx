import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { CampaignType } from "../types";

interface Props {
  onSelect: (type: CampaignType) => void;
}

interface TypeConfig {
  type: CampaignType;
  icon: ReactNode;
  iconBg: string;
  iconFg: string;
}

const TYPES: TypeConfig[] = [
  {
    type: "service",
    icon: <SparklesIcon />,
    iconBg: "bg-pink-50",
    iconFg: "text-pink-500",
  },
  {
    type: "food",
    icon: <UtensilsIcon />,
    iconBg: "bg-amber-50",
    iconFg: "text-amber-600",
  },
  {
    type: "product",
    icon: <PackageIcon />,
    iconBg: "bg-violet-50",
    iconFg: "text-violet-500",
  },
  {
    type: "event",
    icon: <CalendarIcon />,
    iconBg: "bg-emerald-50",
    iconFg: "text-emerald-600",
  },
];

export default function CampaignTypeStep({ onSelect }: Props) {
  const { t } = useTranslation("prototype_campaigns");

  return (
    <div data-testid="campaign-type-step">
      <p className="text-sm text-gray-500">{t("wizard.stepType")}</p>
      <h2 className="mt-1 text-xl font-semibold text-gray-900">
        {t("wizard.typeIntro")}
      </h2>

      <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
        {TYPES.map(({ type, icon, iconBg, iconFg }) => (
          <button
            key={type}
            type="button"
            onClick={() => onSelect(type)}
            className="group flex flex-col items-center gap-4 rounded-2xl border border-surface-200 bg-white px-4 py-7 text-center shadow-sm transition hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md"
            data-testid={`campaign-type-${type}`}
          >
            <span
              className={`flex h-14 w-14 items-center justify-center rounded-2xl ${iconBg} ${iconFg} transition group-hover:scale-105`}
              aria-hidden="true"
            >
              {icon}
            </span>
            <p className="text-sm font-semibold text-gray-900">
              {t(`types.${type}`)}
            </p>
          </button>
        ))}
      </div>
    </div>
  );
}

const ICON_PROPS = {
  width: 26,
  height: 26,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.6,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

function SparklesIcon() {
  // Beauty / care — three stars, lucide style
  return (
    <svg {...ICON_PROPS}>
      <path d="M12 3l1.6 4.6a2 2 0 0 0 1.3 1.3L19.5 10.5l-4.6 1.6a2 2 0 0 0-1.3 1.3L12 18l-1.6-4.6a2 2 0 0 0-1.3-1.3L4.5 10.5l4.6-1.6a2 2 0 0 0 1.3-1.3z" />
      <path d="M19 4v3" />
      <path d="M17.5 5.5h3" />
      <path d="M5 17v3" />
      <path d="M3.5 18.5h3" />
    </svg>
  );
}

function UtensilsIcon() {
  // Fork + knife
  return (
    <svg {...ICON_PROPS}>
      <path d="M7 3v8a2 2 0 0 0 2 2v8" />
      <path d="M11 3v6a2 2 0 0 1-2 2" />
      <path d="M16 3c-1.5 1.5-2.5 3-2.5 5.5S15 12 16.5 12.5V21" />
    </svg>
  );
}

function PackageIcon() {
  // Box / parcel
  return (
    <svg {...ICON_PROPS}>
      <path d="M21 8v8a2 2 0 0 1-1 1.7l-7 4a2 2 0 0 1-2 0l-7-4A2 2 0 0 1 3 16V8a2 2 0 0 1 1-1.7l7-4a2 2 0 0 1 2 0l7 4A2 2 0 0 1 21 8z" />
      <path d="M3.3 7l8.7 5 8.7-5" />
      <path d="M12 22V12" />
      <path d="M7.5 4.5l9 5" />
    </svg>
  );
}

function CalendarIcon() {
  // Calendar with marker
  return (
    <svg {...ICON_PROPS}>
      <rect x="3.5" y="5" width="17" height="15.5" rx="2" />
      <path d="M3.5 10h17" />
      <path d="M8 3v4" />
      <path d="M16 3v4" />
      <circle cx="12" cy="14.5" r="1.2" fill="currentColor" stroke="none" />
    </svg>
  );
}
