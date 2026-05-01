import { type ReactNode } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getCampaign, listCampaignApplications } from "@/_prototype/api/campaigns";
import { campaignKeys } from "@/_prototype/queryKeys";
import { ROUTES } from "@/_prototype/routes";
import Spinner from "@/_prototype/shared/components/Spinner";
import ErrorState from "@/_prototype/shared/components/ErrorState";
import CampaignStatusBadge from "./components/CampaignStatusBadge";
import ApplicationsSection from "./sections/ApplicationsSection";
import SelectedSection from "./sections/SelectedSection";
import ContractSection from "./sections/ContractSection";
import type { Campaign } from "./types";

type Section =
  | "brief"
  | "applications"
  | "selected"
  | "contract"
  | "scenario_in_work"
  | "scenarios_review"
  | "shooting"
  | "materials_review"
  | "rework"
  | "awaiting_publication"
  | "published"
  | "declined"
  | "analytics";

interface SectionDef {
  key: Section;
  group: "prep" | "pipeline" | "analytics";
  enabled: (c: Campaign) => boolean;
}

const SECTIONS: SectionDef[] = [
  { key: "brief", group: "prep", enabled: () => true },
  { key: "applications", group: "prep", enabled: () => true },
  { key: "selected", group: "prep", enabled: () => true },
  { key: "contract", group: "prep", enabled: () => true },
  {
    key: "scenario_in_work",
    group: "pipeline",
    enabled: (c) => c.requiresScriptApproval,
  },
  {
    key: "scenarios_review",
    group: "pipeline",
    enabled: (c) => c.requiresScriptApproval,
  },
  { key: "shooting", group: "pipeline", enabled: () => true },
  {
    key: "materials_review",
    group: "pipeline",
    enabled: (c) => c.requiresMaterialApproval,
  },
  {
    key: "rework",
    group: "pipeline",
    enabled: (c) => c.requiresScriptApproval || c.requiresMaterialApproval,
  },
  { key: "awaiting_publication", group: "pipeline", enabled: () => true },
  { key: "published", group: "pipeline", enabled: () => true },
  { key: "declined", group: "pipeline", enabled: () => true },
  { key: "analytics", group: "analytics", enabled: () => true },
];

export default function CampaignDetailPage() {
  const { t } = useTranslation("prototype_campaigns");
  const { campaignId } = useParams<{ campaignId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const section = (searchParams.get("section") as Section) || "brief";

  const query = useQuery({
    queryKey: campaignKeys.detail(campaignId ?? ""),
    queryFn: () => getCampaign(campaignId ?? ""),
    enabled: !!campaignId,
  });

  const applicationsQuery = useQuery({
    queryKey: campaignKeys.applications(campaignId ?? ""),
    queryFn: () => listCampaignApplications(campaignId ?? ""),
    enabled: !!campaignId,
  });

  if (!campaignId) return <ErrorState message={t("loadError")} />;
  if (query.isLoading) return <Spinner className="mt-12" />;
  if (query.isError || !query.data)
    return (
      <ErrorState
        message={t("loadError")}
        onRetry={() => void query.refetch()}
      />
    );

  const campaign = query.data;
  const applications = applicationsQuery.data ?? [];
  const counts: Record<Section, number> = {
    brief: 0,
    applications: applications.filter((a) => a.status === "new").length,
    selected: applications.filter(
      (a) => a.status === "approved" && a.tzStatus !== "replaced",
    ).length,
    contract: 0,
    scenario_in_work: 0,
    scenarios_review: 0,
    shooting: 0,
    materials_review: 0,
    rework: 0,
    awaiting_publication: 0,
    published: 0,
    declined: 0,
    analytics: 0,
  };
  const sections = SECTIONS.filter((s) => s.enabled(campaign));

  function pickSection(next: Section) {
    const params = new URLSearchParams(searchParams);
    if (next === "brief") params.delete("section");
    else params.set("section", next);
    setSearchParams(params);
  }

  return (
    <div data-testid="campaign-detail-page">
      <Link
        to={"/prototype/" + ROUTES.CAMPAIGNS}
        className="text-sm text-primary hover:underline"
        data-testid="back-to-campaigns"
      >
        ← {t("backToList")}
      </Link>

      <div className="mt-3 flex flex-wrap items-baseline justify-between gap-3">
        <div className="flex flex-wrap items-baseline gap-3">
          <h1 className="text-2xl font-bold text-gray-900">{campaign.title}</h1>
          <span className="text-sm text-gray-500">
            {t(`types.${campaign.type}`)}
          </span>
          <CampaignStatusBadge status={campaign.status} />
        </div>
        {(campaign.status === "draft" || campaign.status === "rejected") && (
          <Link
            to={"/prototype/" + ROUTES.CAMPAIGN_EDIT(campaign.id)}
            className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600"
            data-testid="edit-campaign-button"
          >
            {t("editCampaign")}
          </Link>
        )}
      </div>

      {campaign.status === "rejected" && campaign.rejectionComment && (
        <div className="mt-4 rounded-card border border-red-200 bg-red-50 p-3 text-sm text-red-800">
          <p className="font-semibold">{t("brief.rejectionComment")}:</p>
          <p className="mt-1 whitespace-pre-wrap">{campaign.rejectionComment}</p>
        </div>
      )}

      <div className="mt-6 grid gap-6 md:grid-cols-[220px_1fr]">
        <CampaignSidebar
          sections={sections}
          current={section}
          onPick={pickSection}
          counts={counts}
        />

        <div data-testid={`section-${section}`}>
          {section === "brief" && <BriefTab campaign={campaign} />}
          {section === "applications" && (
            <ApplicationsSection campaignId={campaign.id} />
          )}
          {section === "selected" && (
            <SelectedSection campaignId={campaign.id} />
          )}
          {section === "contract" && (
            <ContractSection campaignId={campaign.id} />
          )}
          {section !== "brief" &&
            section !== "applications" &&
            section !== "selected" &&
            section !== "contract" && (
              <SectionPlaceholder section={section} />
            )}
        </div>
      </div>
    </div>
  );
}

function CampaignSidebar({
  sections,
  current,
  onPick,
  counts,
}: {
  sections: SectionDef[];
  current: Section;
  onPick: (s: Section) => void;
  counts: Record<Section, number>;
}) {
  const { t } = useTranslation("prototype_campaigns");
  const grouped = {
    prep: sections.filter((s) => s.group === "prep"),
    pipeline: sections.filter((s) => s.group === "pipeline"),
    analytics: sections.filter((s) => s.group === "analytics"),
  };
  return (
    <aside className="rounded-2xl border border-surface-200 bg-white p-3 shadow-sm">
      {(["prep", "pipeline", "analytics"] as const).map((g) => (
        <div key={g} className="mb-3 last:mb-0">
          <p className="px-3 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-gray-400">
            {t(`detailGroups.${g}`)}
          </p>
          <ul className="space-y-0.5">
            {grouped[g].map((s) => {
              const count = counts[s.key] ?? 0;
              const active = current === s.key;
              return (
                <li key={s.key}>
                  <button
                    type="button"
                    onClick={() => onPick(s.key)}
                    data-testid={`sidebar-${s.key}`}
                    className={`flex w-full items-center justify-between gap-2 rounded-button px-3 py-2 text-left text-sm transition ${
                      active
                        ? "bg-primary-50 font-medium text-primary"
                        : "text-gray-700 hover:bg-surface-100"
                    }`}
                  >
                    <span>{t(`sections.${s.key}`)}</span>
                    {count > 0 && (
                      <span
                        className={`rounded-full px-2 py-0.5 text-xs ${
                          active
                            ? "bg-primary text-white"
                            : "bg-surface-200 text-gray-600"
                        }`}
                      >
                        {count}
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </aside>
  );
}

function SectionPlaceholder({ section }: { section: Section }) {
  const { t } = useTranslation("prototype_campaigns");
  return (
    <div className="rounded-2xl border border-dashed border-surface-300 bg-white p-8 text-center">
      <p className="text-base font-semibold text-gray-900">
        {t(`sections.${section}`)}
      </p>
      <p className="mt-2 text-sm text-gray-500">
        {t(`sectionDescriptions.${section}`)}
      </p>
      <p className="mt-3 text-xs text-gray-400">{t("comingSoon")}</p>
    </div>
  );
}

function BriefTab({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("prototype_campaigns");
  const isInstagram = campaign.socialPlatform === "instagram";

  const showTypeSpecific =
    campaign.type !== "event" &&
    ((campaign.requiresVisit && campaign.visitDetails) ||
      campaign.halal !== undefined);

  return (
    <div className="space-y-6">
      {/* Event-specific parameters (event type) */}
      {campaign.eventDetails && (
        <BriefSection title={t("form.sections.event")}>
          <EventDetailsBlock event={campaign.eventDetails} />
        </BriefSection>
      )}

      {/* Type-specific block (visit / halal for non-event types) */}
      {showTypeSpecific && (
        <BriefSection title={t("form.sections.typeSpecific")}>
          {campaign.requiresVisit && campaign.visitDetails && (
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                {t("brief.visit")}
              </p>
              <p className="mt-1 text-sm text-gray-900">
                {campaign.visitDetails.city}, {campaign.visitDetails.address}
              </p>
              <p className="mt-1 text-sm text-gray-500">
                {campaign.visitDetails.slots.length === 0
                  ? t("brief.visitAnyTime")
                  : campaign.visitDetails.slots
                      .map((s) => `${s.date} ${s.time}`)
                      .join(", ")}
              </p>
            </div>
          )}
          {campaign.halal !== undefined && (
            <BriefRow label={t("brief.halal")}>
              <span className="text-sm text-gray-900">
                {campaign.halal ? t("yes") : t("no")}
              </span>
            </BriefRow>
          )}
        </BriefSection>
      )}

      {/* Audience — key numbers on top, attributes and demographics below */}
      <BriefSection title={t("form.sections.audience")}>
        <div className="grid gap-3 sm:grid-cols-3">
          <BriefRow label={t("brief.creatorsCount")}>
            <span className="text-base font-semibold tabular-nums text-gray-900">
              {campaign.creatorsCount}
            </span>
          </BriefRow>
          <BriefRow label={t("brief.minFollowers")}>
            <span className="text-base font-semibold tabular-nums text-gray-900">
              {campaign.minFollowers
                ? campaign.minFollowers.toLocaleString("ru")
                : "—"}
            </span>
          </BriefRow>
          <BriefRow label={t("brief.minAvgViews")}>
            <span className="text-base font-semibold tabular-nums text-gray-900">
              {campaign.minAvgViews
                ? campaign.minAvgViews.toLocaleString("ru")
                : "—"}
            </span>
          </BriefRow>
        </div>

        <BriefRow label={t("brief.categories")}>
          {campaign.anyCategories ? (
            <span className="text-sm text-gray-700">
              {t("form.categoriesAny")}
            </span>
          ) : (
            <ChipList
              items={campaign.categories.map((c) => c.name)}
              emptyText="—"
            />
          )}
        </BriefRow>
        <BriefRow label={t("brief.cities")}>
          {campaign.anyCities ? (
            <span className="text-sm text-gray-700">
              {t("form.citiesAny")}
            </span>
          ) : (
            <ChipList
              items={campaign.cities.map((c) => c.name)}
              emptyText="—"
            />
          )}
        </BriefRow>

        <div className="flex flex-wrap gap-x-8 gap-y-3">
          <BriefRow label={t("brief.age")}>
            <span className="text-sm text-gray-900">
              {formatAge(campaign)}
            </span>
          </BriefRow>
          <BriefRow label={t("brief.genders")}>
            <ChipList
              items={campaign.genders.map((g) => t(`gender.${g}`))}
              emptyText="—"
            />
          </BriefRow>
        </div>
      </BriefSection>

      {/* Content */}
      <BriefSection title={t("form.sections.content")}>
        <div className="grid gap-3 sm:grid-cols-2">
          <BriefRow label={t("brief.socialPlatform")}>
            <span className="text-sm font-medium text-gray-900">
              {t(`social.${campaign.socialPlatform}`)}
            </span>
          </BriefRow>
          <BriefRow label={t("form.publicationMode")}>
            <span className="text-sm text-gray-900">
              {t(
                campaign.publicationMode === "brand_only"
                  ? "form.publicationModeBrandOnly"
                  : "form.publicationModeCreator",
              )}
            </span>
          </BriefRow>
        </div>
        {isInstagram && (
          <BriefRow label={t("brief.contentFormats")}>
            <ChipList
              items={campaign.contentFormats.map((f) => t(`format.${f}`))}
              emptyText="—"
            />
          </BriefRow>
        )}
        <BriefRow label={t("brief.postsPerCreator")}>
          <span className="text-sm text-gray-900">
            {formatPostsByFormat(campaign.postsByFormat, t)}
          </span>
        </BriefRow>
        <BriefRow label={t("brief.languages")}>
          <ChipList
            items={campaign.languages.map((l) => t(`language.${l}`))}
            emptyText="—"
          />
        </BriefRow>
        <BriefRow label={t("brief.publishDeadline")}>
          <span className="text-sm text-gray-900">
            {campaign.publishDeadlineMode === "exact"
              ? `${t("form.publishDeadlineExactLabel")}: ${formatDate(campaign.publishDeadline)}`
              : `${t("form.publishDeadlineUntilLabel")} ${formatDate(campaign.publishDeadline)}`}
          </span>
        </BriefRow>
        {campaign.crossposting !== "creator_choice" && (
          <BriefRow label={t("form.crossposting")}>
            <span className="text-sm text-gray-900">
              {t(
                campaign.crossposting === "to_instagram"
                  ? "form.crosspostingToInstagram"
                  : "form.crosspostingToTiktok",
              )}
            </span>
          </BriefRow>
        )}
      </BriefSection>

      {/* Brief and materials */}
      <BriefSection title={t("form.sections.brief")}>
        <BriefRow label={t("brief.description")}>
          <p className="whitespace-pre-wrap text-sm text-gray-900">
            {campaign.description}
          </p>
        </BriefRow>
        {campaign.references.length > 0 && (
          <BriefRow label={t("brief.references")}>
            <ul className="space-y-2">
              {campaign.references.map((ref, i) => (
                <li
                  key={`${ref.url}-${i}`}
                  className="rounded-card border border-surface-200 bg-surface-50 p-3"
                >
                  <a
                    href={ref.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="break-all text-sm text-primary hover:underline"
                  >
                    {ref.url}
                  </a>
                  {ref.description && (
                    <p className="mt-1 text-sm text-gray-700">
                      {ref.description}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          </BriefRow>
        )}
        {(campaign.hashtags ||
          campaign.mentionsInCaption ||
          campaign.mentionsInPublication ||
          campaign.adDisclaimer) && (
          <BriefRow label={t("brief.contentRequirements")}>
            <ContentRequirementsBlock campaign={campaign} />
          </BriefRow>
        )}
        {campaign.attachments.length > 0 && (
          <BriefRow label={t("brief.attachments")}>
            <ul className="space-y-1.5">
              {campaign.attachments.map((file) => (
                <li
                  key={file.id}
                  className="flex items-center gap-3 rounded-button border border-surface-200 bg-surface-50 px-3 py-2"
                >
                  <FileIcon contentType={file.contentType} />
                  <div className="min-w-0 flex-1">
                    <a
                      href={file.url}
                      download={file.name}
                      className="block truncate text-sm font-medium text-primary hover:underline"
                    >
                      {file.name}
                    </a>
                    <p className="text-xs text-gray-500">
                      {formatFileSize(file.sizeBytes)}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          </BriefRow>
        )}
      </BriefSection>

      {/* Approvals — show only enabled items */}
      <ApprovalsSection campaign={campaign} />

      {/* Payment */}
      <BriefSection title={t("brief.payment")}>
        <PaymentSummary campaign={campaign} />
      </BriefSection>
    </div>
  );
}

function ApprovalsSection({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("prototype_campaigns");
  const items: string[] = [];
  if (campaign.requiresScriptApproval) items.push(t("brief.scriptApproval"));
  if (campaign.requiresMaterialApproval)
    items.push(t("brief.materialApproval"));
  if (campaign.isCollabPost) {
    const handles =
      campaign.collabBrandHandles.length > 0
        ? " — " +
          campaign.collabBrandHandles.map((h) => `@${h}`).join(", ")
        : "";
    items.push(t("form.collabPost") + handles);
  }
  if (items.length === 0) return null;

  return (
    <BriefSection title={t("brief.approvals")}>
      <ul className="space-y-2 text-sm">
        {items.map((label) => (
          <li key={label}>
            <ApprovalLine label={label} value />
          </li>
        ))}
      </ul>
    </BriefSection>
  );
}

function BriefSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-2xl border border-surface-200 bg-white p-5 shadow-sm">
      <h3 className="mb-4 border-b border-surface-200 pb-3 text-base font-semibold text-gray-900">
        {title}
      </h3>
      <div className="space-y-3">{children}</div>
    </section>
  );
}

function BriefRow({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div>
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </p>
      <div className="mt-1">{children}</div>
    </div>
  );
}

function formatAge(c: Campaign): string {
  if (!c.ageMin && !c.ageMax) return "—";
  return `${c.ageMin ?? "?"}–${c.ageMax ?? "?"}`;
}

function ChipList({ items, emptyText }: { items: string[]; emptyText: string }) {
  if (items.length === 0) return <span className="text-gray-400">{emptyText}</span>;
  return (
    <div className="flex flex-wrap gap-1.5">
      {items.map((label) => (
        <span
          key={label}
          className="inline-flex items-center rounded-full border border-surface-300 bg-surface-100 px-2.5 py-0.5 text-xs text-gray-800"
        >
          {label}
        </span>
      ))}
    </div>
  );
}

function ApprovalLine({ label, value }: { label: string; value: boolean }) {
  return (
    <span className={value ? "text-emerald-700" : "text-gray-500"}>
      {value ? "✓" : "—"} {label}
    </span>
  );
}

function FileIcon({ contentType }: { contentType: string }) {
  const isImage = contentType.startsWith("image/");
  return (
    <svg
      className="h-5 w-5 shrink-0 text-gray-500"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {isImage ? (
        <>
          <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
          <circle cx="8.5" cy="8.5" r="1.5" />
          <polyline points="21 15 16 10 5 21" />
        </>
      ) : (
        <>
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
        </>
      )}
    </svg>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

function formatPostsByFormat(
  posts: Campaign["postsByFormat"],
  t: (key: string) => string,
): ReactNode {
  const entries = Object.entries(posts).filter(([, n]) => n && n > 0);
  if (entries.length === 0) return "—";
  return entries
    .map(([fmt, n]) => `${n} × ${t(`format.${fmt}`)}`)
    .join(", ");
}

function ContentRequirementsBlock({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("prototype_campaigns");
  const items: { label: string; value: string }[] = [];
  if (campaign.hashtags) items.push({ label: t("form.hashtags"), value: campaign.hashtags });
  if (campaign.mentionsInCaption)
    items.push({ label: t("form.mentionsInCaption"), value: campaign.mentionsInCaption });
  if (campaign.mentionsInPublication)
    items.push({ label: t("form.mentionsInPublication"), value: campaign.mentionsInPublication });
  if (campaign.adDisclaimer) {
    const placement = campaign.adDisclaimerPlacement
      ? t(
          campaign.adDisclaimerPlacement === "caption"
            ? "form.adDisclaimerPlacementCaption"
            : campaign.adDisclaimerPlacement === "publication"
              ? "form.adDisclaimerPlacementPublication"
              : "form.adDisclaimerPlacementBoth",
        )
      : "";
    items.push({
      label: t("form.adDisclaimer"),
      value: placement
        ? `${campaign.adDisclaimer} (${placement})`
        : campaign.adDisclaimer,
    });
  }
  if (items.length === 0) return <span className="text-sm text-gray-500">—</span>;
  return (
    <ul className="space-y-1 text-sm">
      {items.map((it) => (
        <li key={it.label}>
          <span className="font-medium text-gray-900">{it.label}:</span>{" "}
          <span className="text-gray-700">{it.value}</span>
        </li>
      ))}
    </ul>
  );
}

function EventDetailsBlock({ event }: { event: NonNullable<Campaign["eventDetails"]> }) {
  const { t } = useTranslation("prototype_campaigns");
  return (
    <div className="space-y-1 text-sm">
      <p className="text-gray-900">
        {event.country}, {event.city}, {event.address}
      </p>
      <p className="text-gray-500">
        {formatDate(event.date)}, {event.timeFrom}–{event.timeTo}
      </p>
      {event.dressCode && (
        <p className="text-gray-500">
          {t("brief.eventDressCode")}: {event.dressCode}
        </p>
      )}
      {event.parking && (
        <p className="text-gray-500">
          {t("brief.eventParking")}:{" "}
          {t(
            event.parking === "paid"
              ? "form.eventParkingPaid"
              : event.parking === "free"
                ? "form.eventParkingFree"
                : "form.eventParkingNone",
          )}
        </p>
      )}
      {event.entryInstructions && (
        <p className="whitespace-pre-wrap text-gray-700">
          {t("brief.eventEntryInstructions")}: {event.entryInstructions}
        </p>
      )}
      {event.transfer && (
        <p className="text-gray-700">
          {t("brief.eventTransfer")}:{" "}
          {t(
            event.transfer.type === "personal"
              ? "form.eventTransferPersonal"
              : "form.eventTransferGroup",
          )}
          {event.transfer.type === "group" && (
            <>
              {" — "}
              {event.transfer.pickup}, {event.transfer.schedule}
            </>
          )}
        </p>
      )}
    </div>
  );
}

function PaymentSummary({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("prototype_campaigns");
  if (campaign.paymentType === "creator_proposal") {
    return <span className="text-sm">{t("brief.paymentCreatorProposal")}</span>;
  }
  return (
    <div className="space-y-2 text-sm">
      {(campaign.paymentType === "barter" ||
        campaign.paymentType === "barter_fixed") && (
        <div>
          <div>
            <span className="font-medium text-gray-900">
              {t("brief.paymentBarterDescription")}:
            </span>{" "}
            <span className="whitespace-pre-wrap">
              {campaign.barterDescription ?? "—"}
            </span>
            {campaign.barterValue !== undefined && (
              <span className="ml-1 text-gray-500">
                · ~{campaign.barterValue.toLocaleString("ru")} ₸
              </span>
            )}
          </div>
          {campaign.barterAttachments.length > 0 && (
            <ul className="mt-2 grid grid-cols-3 gap-2">
              {campaign.barterAttachments.map((file) => (
                <li
                  key={file.id}
                  className="overflow-hidden rounded-card border border-surface-200 bg-surface-100"
                >
                  {file.contentType.startsWith("image/") ? (
                    <a href={file.url} target="_blank" rel="noopener noreferrer">
                      <img
                        src={file.url}
                        alt={file.name}
                        className="h-24 w-full object-cover"
                      />
                    </a>
                  ) : (
                    <a
                      href={file.url}
                      download={file.name}
                      className="block truncate p-2 text-xs text-primary hover:underline"
                    >
                      {file.name}
                    </a>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {(campaign.paymentType === "fixed" ||
        campaign.paymentType === "barter_fixed") && (
        <div>
          <span className="font-medium text-gray-900">
            {t("brief.paymentFixed")}:
          </span>{" "}
          {(campaign.paymentAmount ?? 0).toLocaleString("ru")} ₸
        </div>
      )}
    </div>
  );
}
