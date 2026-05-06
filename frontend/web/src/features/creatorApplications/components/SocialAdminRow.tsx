import { useTranslation } from "react-i18next";
import type { components } from "@/api/generated/schema";
import SocialLink from "@/shared/components/SocialLink";
import { PLATFORM_LABELS } from "@/shared/constants/socials";

type DetailSocial = components["schemas"]["CreatorApplicationDetailSocial"];

interface SocialAdminRowProps {
  social: DetailSocial;
  telegramLinked: boolean;
  onVerifyClick: (social: DetailSocial) => void;
}

export default function SocialAdminRow({
  social,
  telegramLinked,
  onVerifyClick,
}: SocialAdminRowProps) {
  const { t } = useTranslation("creatorApplications");

  return (
    <div
      className="flex items-center justify-between gap-3 py-1"
      data-testid={`social-admin-row-${social.id}`}
    >
      <SocialLink platform={social.platform} handle={social.handle} showHandle />
      {social.verified ? (
        <VerifiedBadge social={social} />
      ) : telegramLinked ? (
        <button
          type="button"
          onClick={() => onVerifyClick(social)}
          data-testid={`verify-social-${social.id}`}
          className="inline-flex items-center rounded-button border border-surface-300 bg-white px-3 py-1 text-xs font-medium text-gray-700 transition hover:bg-surface-100"
        >
          {t("actions.verifyManual")}
        </button>
      ) : (
        <div className="flex items-center gap-2">
          <button
            type="button"
            disabled
            data-testid={`verify-social-${social.id}`}
            className="inline-flex cursor-not-allowed items-center rounded-button border border-surface-300 bg-white px-3 py-1 text-xs font-medium text-gray-400"
          >
            {t("actions.verifyManual")}
          </button>
          <span
            className="text-xs text-gray-500"
            data-testid={`verify-social-${social.id}-disabled-hint`}
          >
            {t("verifyDialog.tgRequired")}
          </span>
        </div>
      )}
    </div>
  );
}

function VerifiedBadge({ social }: { social: DetailSocial }) {
  const { t } = useTranslation("creatorApplications");
  const labelKey = social.method === "manual" ? "verifiedManual" : "verifiedAuto";
  const platformLabel = PLATFORM_LABELS[social.platform];
  const title = social.verifiedAt
    ? `${platformLabel} · ${new Date(social.verifiedAt).toLocaleString("ru")}`
    : platformLabel;
  return (
    <span
      data-testid={`verified-badge-${social.id}`}
      title={title}
      className="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700"
    >
      {t(labelKey)}
    </span>
  );
}
