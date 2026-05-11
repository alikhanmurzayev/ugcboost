import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import CategoryChip from "@/shared/components/CategoryChip";
import SocialLink from "@/shared/components/SocialLink";
import { calcAge } from "@/shared/utils/age";
import {
  CAMPAIGN_CREATOR_DRAWER_GROUPS,
  type CreatorDrawerGroupKey,
} from "@/shared/constants/campaignCreatorStatus";
import { ROUTES } from "@/shared/constants/routes";
import type { components } from "@/api/generated/schema";
import type { CreatorListItem, CreatorAggregate } from "./types";

type CreatorCampaignBrief = components["schemas"]["CreatorCampaignBrief"];
type CampaignCreatorStatus = components["schemas"]["CampaignCreatorStatus"];

interface CreatorDrawerBodyProps {
  prefill: CreatorListItem | undefined;
  detail: CreatorAggregate | undefined;
}

export default function CreatorDrawerBody({
  prefill,
  detail,
}: CreatorDrawerBodyProps) {
  const { t } = useTranslation("creators");

  // Once detail resolves it is the source of truth for nullable fields —
  // a `null` in detail must not fall back to the (possibly stale) prefill.
  const middleName = detail
    ? (detail.middleName ?? null)
    : (prefill?.middleName ?? null);
  const telegramUsername = detail
    ? (detail.telegramUsername ?? null)
    : (prefill?.telegramUsername ?? null);

  const iin = detail?.iin ?? prefill?.iin ?? "";
  const birthDate = detail?.birthDate ?? prefill?.birthDate ?? "";
  const phone = detail?.phone ?? prefill?.phone ?? "";
  const cityName = detail?.cityName ?? prefill?.city.name ?? "";
  const categoriesList: { code: string; name: string }[] =
    detail?.categories ?? prefill?.categories ?? [];
  const socials: { platform: "instagram" | "tiktok" | "threads"; handle: string }[] =
    detail?.socials ?? prefill?.socials ?? [];
  const createdAt = detail?.createdAt ?? prefill?.createdAt ?? "";

  const age = birthDate ? calcAge(birthDate) : 0;

  return (
    <>
      <div
        className="space-y-0.5 text-xs text-gray-500"
        data-testid="creator-timeline"
      >
        {createdAt && (
          <p>
            {t("drawer.approvedAt")}: {formatDateTime(createdAt)}
          </p>
        )}
      </div>

      <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4">
        <Field
          testid="drawer-birth-date"
          label={t("drawer.birthDate")}
          value={
            birthDate
              ? `${formatDate(birthDate)} · ${t("drawer.years", { count: age })}`
              : "—"
          }
        />
        <Field
          testid="drawer-iin"
          label={t("drawer.iin")}
          value={<CopyValue text={iin} testid="drawer-iin-copy" />}
        />
        <Field
          testid="drawer-phone"
          label={t("drawer.phone")}
          value={
            <span className="inline-flex items-center gap-2">
              <a
                href={`tel:${phone}`}
                className="text-primary hover:underline"
                data-testid="creator-phone"
              >
                {phone}
              </a>
              <CopyButton value={phone} testid="drawer-phone-copy" />
            </span>
          }
        />
        <Field
          testid="drawer-city"
          label={t("drawer.city")}
          value={cityName}
        />

        {middleName && (
          <Field
            testid="drawer-middle-name"
            label={t("drawer.middleName")}
            value={middleName}
            fullWidth
          />
        )}

        {detail?.address && (
          <Field
            testid="drawer-address"
            label={t("drawer.address")}
            value={detail.address}
            fullWidth
          />
        )}

        <Field
          fullWidth
          label={t("drawer.categories")}
          value={
            <div className="flex flex-wrap gap-1.5" data-testid="drawer-categories">
              {categoriesList.map((c) => (
                <CategoryChip
                  key={c.code}
                  testid={`drawer-category-${c.code}`}
                >
                  {c.name}
                </CategoryChip>
              ))}
              {detail?.categoryOtherText && (
                <CategoryChip testid="drawer-category-other-text">
                  <span className="italic">
                    {t("drawer.categoryOther")}: {detail.categoryOtherText}
                  </span>
                </CategoryChip>
              )}
            </div>
          }
        />

        <Field
          fullWidth
          label={t("drawer.socials")}
          value={
            <div className="flex flex-col gap-1" data-testid="drawer-socials">
              {socials.map((s) => (
                <SocialLink
                  key={`${s.platform}-${s.handle}`}
                  platform={s.platform}
                  handle={s.handle}
                  showHandle
                />
              ))}
            </div>
          }
        />

        <Field
          fullWidth
          label={t("drawer.telegram")}
          value={
            <CopyValue
              text={renderTelegram(telegramUsername, detail)}
              testid="drawer-telegram-copy"
            />
          }
        />

        {detail?.sourceApplicationId && (
          <Field
            fullWidth
            testid="drawer-source-application-id"
            label={t("drawer.sourceApplicationId")}
            value={
              <span className="font-mono text-xs text-gray-500">
                {detail.sourceApplicationId}
              </span>
            }
          />
        )}
      </dl>

      <CampaignsBlock campaigns={detail?.campaigns ?? []} />
    </>
  );
}

function CampaignsBlock({
  campaigns,
}: {
  campaigns: CreatorCampaignBrief[];
}) {
  const { t } = useTranslation("creators");
  const { t: tCampaigns } = useTranslation("campaigns");

  return (
    <section className="mt-6" data-testid="drawer-campaigns">
      <h3 className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {t("drawer.campaignsTitle")}
      </h3>

      {campaigns.length === 0 ? (
        <p
          className="mt-2 text-sm text-gray-500"
          data-testid="drawer-campaigns-empty"
        >
          {t("drawer.campaignsEmpty")}
        </p>
      ) : (
        <div className="mt-2 space-y-4">
          {CAMPAIGN_CREATOR_DRAWER_GROUPS.map((group) => {
            const rows = filterCampaignsForGroup(campaigns, group.statuses);
            if (rows.length === 0) return null;
            return (
              <div
                key={group.groupKey}
                data-testid={`drawer-campaigns-group-${group.groupKey}`}
              >
                <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-400">
                  {t(`drawer.campaignsGroups.${group.groupKey}` satisfies `drawer.campaignsGroups.${CreatorDrawerGroupKey}`)}
                </h4>
                <ul className="mt-1 flex flex-col gap-1">
                  {rows.map((c) => (
                    <li key={c.id}>
                      <Link
                        to={`/${ROUTES.CAMPAIGN_DETAIL(c.id)}`}
                        className="flex items-center justify-between rounded-button px-2 py-1.5 text-sm text-gray-900 transition hover:bg-surface-100"
                        data-testid={`drawer-campaign-${c.id}`}
                      >
                        <span className="truncate font-medium">{c.name}</span>
                        <span className="ml-3 shrink-0 text-xs text-gray-500">
                          {tCampaigns(
                            `campaignCreators.currentStatus.${c.status}` satisfies `campaignCreators.currentStatus.${CampaignCreatorStatus}`,
                          )}
                        </span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

function filterCampaignsForGroup(
  campaigns: CreatorCampaignBrief[],
  statuses: readonly CampaignCreatorStatus[],
): CreatorCampaignBrief[] {
  const allowed = new Set<CampaignCreatorStatus>(statuses);
  return campaigns.filter((c) => allowed.has(c.status));
}

function renderTelegram(
  username: string | null | undefined,
  detail: CreatorAggregate | undefined,
): string {
  if (username) return `@${username}`;
  if (detail) {
    const fullName = [detail.telegramFirstName, detail.telegramLastName]
      .filter(Boolean)
      .join(" ");
    if (fullName) return fullName;
    return `id ${detail.telegramUserId}`;
  }
  return "—";
}

function CopyValue({ text, testid }: { text: string; testid?: string }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className="text-sm text-gray-900">{text}</span>
      <CopyButton value={text} testid={testid} />
    </span>
  );
}

function CopyButton({ value, testid }: { value: string; testid?: string }) {
  const { t } = useTranslation("creators");
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard может быть недоступен (insecure context / старый браузер).
    }
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={copied ? t("copy.copied") : t("copy.copy")}
      title={copied ? t("copy.copied") : t("copy.copy")}
      data-testid={testid}
      className={`rounded-button p-1 transition ${
        copied
          ? "text-emerald-600"
          : "text-gray-400 hover:bg-surface-200 hover:text-gray-700"
      }`}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
    </button>
  );
}

function CopyIcon() {
  return (
    <svg
      className="h-3.5 w-3.5"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg
      className="h-3.5 w-3.5"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function Field({
  label,
  value,
  fullWidth = false,
  testid,
}: {
  label: string;
  value: ReactNode;
  fullWidth?: boolean;
  testid?: string;
}) {
  return (
    <div className={fullWidth ? "col-span-2" : ""}>
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd className="mt-1 text-sm text-gray-900" data-testid={testid}>
        {value}
      </dd>
    </div>
  );
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru");
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("ru", {
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
