import { type ChangeEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { listDictionary } from "@/api/dictionaries";
import { dictionaryKeys } from "@/shared/constants/queryKeys";
import CurrencyInput from "./CurrencyInput";
import DatePicker from "./DatePicker";
import MultiselectWithAll from "./MultiselectWithAll";
import Select from "./Select";

const COUNTRIES: { code: string; name: string }[] = [
  { code: "kz", name: "Казахстан" },
  { code: "ru", name: "Россия" },
  { code: "uz", name: "Узбекистан" },
  { code: "kg", name: "Кыргызстан" },
  { code: "by", name: "Беларусь" },
  { code: "ge", name: "Грузия" },
  { code: "am", name: "Армения" },
  { code: "az", name: "Азербайджан" },
  { code: "tr", name: "Турция" },
  { code: "ae", name: "ОАЭ" },
];

function findCountryCode(name: string): string | undefined {
  return COUNTRIES.find((c) => c.name === name)?.code;
}
import type {
  Attachment,
  CampaignType,
  ContentFormat,
  EventDetails,
  EventParking,
  EventTransfer,
  EventTransferType,
  Gender,
  Language,
  PaymentType,
  Reference,
  SocialPlatform,
  VisitDetails,
} from "../types";

export interface CampaignFormValues {
  title: string;
  description: string;
  hashtags?: string;
  mentionsInCaption?: string;
  mentionsInPublication?: string;
  adDisclaimer?: string;
  adDisclaimerPlacement?: "caption" | "publication" | "both";
  references: Reference[];
  attachments: Attachment[];
  creatorsCount: number;
  anyCategories: boolean;
  categoryCodes: string[];
  minFollowers: number;
  minAvgViews: number;
  ageMin?: number;
  ageMax?: number;
  anyCities: boolean;
  cityCodes: string[];
  genders: Gender[];
  languages: Language[];
  socialPlatform?: SocialPlatform;
  contentFormats: ContentFormat[];
  postsByFormat: Partial<Record<ContentFormat, number>>;
  publishDeadline: string;
  publishDeadlineMode?: "until" | "exact";
  paymentType?: PaymentType;
  paymentAmount?: number;
  barterDescription?: string;
  barterValue?: number;
  barterAttachments: Attachment[];
  publicationMode?: "creator" | "brand_only";
  isCollabPost: boolean;
  collabBrandHandles: string[];
  crossposting?: "creator_choice" | "to_instagram" | "to_tiktok";
  requiresScriptApproval: boolean;
  requiresMaterialApproval: boolean;
  requiresVisit: boolean;
  visitDetails?: VisitDetails;
  eventDetails?: EventDetails;
  halal?: boolean;
}

interface Props {
  type: CampaignType;
  values: CampaignFormValues;
  errors: Partial<Record<keyof CampaignFormValues, string>>;
  onChange: <K extends keyof CampaignFormValues>(
    key: K,
    value: CampaignFormValues[K],
  ) => void;
}

export default function CampaignForm({ type, values, errors, onChange }: Props) {
  const { t } = useTranslation("campaigns");
  const citiesQuery = useQuery({
    queryKey: dictionaryKeys.list("cities"),
    queryFn: () => listDictionary("cities"),
    staleTime: 5 * 60 * 1000,
  });
  const categoriesQuery = useQuery({
    queryKey: dictionaryKeys.list("categories"),
    queryFn: () => listDictionary("categories"),
    staleTime: 5 * 60 * 1000,
  });

  function handleNum(key: keyof CampaignFormValues) {
    return (e: ChangeEvent<HTMLInputElement>) => {
      const v = e.target.value;
      onChange(key, (v ? Number(v) : undefined) as never);
    };
  }

  function toggleSet<T extends string>(arr: T[], item: T): T[] {
    return arr.includes(item) ? arr.filter((x) => x !== item) : [...arr, item];
  }

  function toggleFormat(f: ContentFormat) {
    const next = toggleSet(values.contentFormats, f);
    onChange("contentFormats", next);
    if (!next.includes(f)) {
      const counts = { ...values.postsByFormat };
      delete counts[f];
      onChange("postsByFormat", counts);
    }
  }

  const isInstagram = values.socialPlatform === "instagram";

  return (
    <div className="space-y-8">
      <Section title={t("form.sections.main")}>
        <FieldRow
          label={t(`form.titleLabels.${type}`)}
          error={errors.title}
          required
        >
          <input
            type="text"
            value={values.title}
            onChange={(e) => onChange("title", e.target.value)}
            placeholder={t(`form.titlePlaceholders.${type}`)}
            className={inputCls}
            data-testid="form-title"
          />
        </FieldRow>

        {type === "event" && (
          <CheckboxRow
            label={t("form.requiresVisit")}
            checked={values.requiresVisit}
            onChange={(v) => onChange("requiresVisit", v)}
            testid="form-event-requires-visit"
          />
        )}
      </Section>

      {type === "event" && (
        <Section title={t("form.sections.event")}>
          <EventDetailsFields
            requiresVisit={values.requiresVisit}
            value={
              values.eventDetails ?? {
                country: "Казахстан",
                city: "",
                date: "",
                timeFrom: "",
                timeTo: "",
                address: "",
              }
            }
            onChange={(v) => onChange("eventDetails", v)}
            error={errors.eventDetails}
          />
        </Section>
      )}

      {type !== "event" && (
        <Section title={t("form.sections.typeSpecific")}>
          <CheckboxRow
            label={t("form.requiresVisit")}
            checked={values.requiresVisit}
            onChange={(v) => {
              onChange("requiresVisit", v);
              if (!v) onChange("visitDetails", undefined);
              else if (!values.visitDetails) {
                onChange("visitDetails", {
                  city: "",
                  address: "",
                  slots: [],
                });
              }
            }}
            testid="form-requires-visit"
          />

          {values.requiresVisit && values.visitDetails && (
            <VisitDetailsField
              visit={values.visitDetails}
              onChange={(v) => onChange("visitDetails", v)}
            />
          )}

          {(type === "food" || (type === "service" && values.requiresVisit)) && (
            <FieldRow
              label={t("form.halal")}
              error={errors.halal}
              required={type === "food"}
            >
              <div className="flex gap-3">
                <RadioChip
                  checked={values.halal === true}
                  onChange={() => onChange("halal", true)}
                  label={t("yes")}
                  testid="form-halal-yes"
                />
                <RadioChip
                  checked={values.halal === false}
                  onChange={() => onChange("halal", false)}
                  label={t("no")}
                  testid="form-halal-no"
                />
              </div>
            </FieldRow>
          )}
        </Section>
      )}

      <Section title={t("form.sections.audience")}>
        <FieldRow
          label={t("form.creatorsCount")}
          error={errors.creatorsCount}
          required
        >
          <input
            type="number"
            min={1}
            value={values.creatorsCount || ""}
            onChange={handleNum("creatorsCount")}
            className={`${inputCls} w-32`}
            data-testid="form-creators-count"
          />
        </FieldRow>

        <FieldRow
          label={t("form.categories")}
          error={errors.categoryCodes}
          required
        >
          <MultiselectWithAll
            options={categoriesQuery.data ?? []}
            selected={values.categoryCodes}
            any={values.anyCategories}
            onChange={({ any, selected }) => {
              onChange("anyCategories", any);
              onChange("categoryCodes", selected);
            }}
            placeholder={t("form.categories")}
            searchPlaceholder={t("form.categories")}
            isLoading={categoriesQuery.isLoading}
            allLabel={t("form.categoriesAny")}
            testid="form-categories"
          />
        </FieldRow>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <FieldRow label={t("form.minFollowers")}>
            <input
              type="number"
              min={0}
              value={values.minFollowers || ""}
              onChange={handleNum("minFollowers")}
              className={`${inputCls} w-40`}
              data-testid="form-min-followers"
            />
          </FieldRow>
          <FieldRow label={t("form.minAvgViews")}>
            <input
              type="number"
              min={0}
              value={values.minAvgViews || ""}
              onChange={handleNum("minAvgViews")}
              className={`${inputCls} w-40`}
              data-testid="form-min-avg-views"
            />
          </FieldRow>
        </div>

        <FieldRow label={t("form.age")}>
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-500">{t("form.ageFrom")}</span>
            <input
              type="number"
              min={14}
              max={100}
              value={values.ageMin ?? ""}
              onChange={handleNum("ageMin")}
              className={`${inputCls} w-20`}
              data-testid="form-age-min"
            />
            <span className="text-sm text-gray-500">{t("form.ageTo")}</span>
            <input
              type="number"
              min={14}
              max={100}
              value={values.ageMax ?? ""}
              onChange={handleNum("ageMax")}
              className={`${inputCls} w-20`}
              data-testid="form-age-max"
            />
          </div>
        </FieldRow>

        <FieldRow
          label={t("form.cities")}
          error={errors.cityCodes}
          required
        >
          <MultiselectWithAll
            options={citiesQuery.data ?? []}
            selected={values.cityCodes}
            any={values.anyCities}
            onChange={({ any, selected }) => {
              onChange("anyCities", any);
              onChange("cityCodes", selected);
            }}
            placeholder={t("form.cities")}
            searchPlaceholder={t("form.cities")}
            isLoading={citiesQuery.isLoading}
            allLabel={t("form.citiesAny")}
            testid="form-cities"
          />
        </FieldRow>

        <FieldRow label={t("form.genders")} error={errors.genders} required>
          <div className="flex gap-3">
            {(["male", "female"] as Gender[]).map((g) => (
              <CheckboxChip
                key={g}
                checked={values.genders.includes(g)}
                onChange={() =>
                  onChange("genders", toggleSet(values.genders, g))
                }
                label={t(`gender.${g}`)}
                testid={`form-gender-${g}`}
              />
            ))}
          </div>
        </FieldRow>
      </Section>

      <Section title={t("form.sections.content")}>
        <FieldRow
          label={t("form.socialPlatform")}
          error={errors.socialPlatform}
          required
        >
          <div className="flex gap-3">
            {(["instagram", "tiktok", "threads"] as SocialPlatform[]).map((p) => (
              <RadioChip
                key={p}
                checked={values.socialPlatform === p}
                onChange={() => {
                  if (values.socialPlatform === p) return;
                  onChange("socialPlatform", p);
                  // Reset dependent radios so the user has to choose again.
                  onChange("crossposting", undefined);
                  if (p === "threads") {
                    // Threads supports only on-creator publishing, no collab.
                    onChange("publicationMode", "creator");
                    onChange("isCollabPost", false);
                    onChange("collabBrandHandles", []);
                    onChange("crossposting", "creator_choice");
                  } else {
                    onChange("publicationMode", undefined);
                  }
                  if (p !== "instagram") {
                    onChange("isCollabPost", false);
                    onChange("collabBrandHandles", []);
                  }
                }}
                label={t(`social.${p}`)}
                testid={`form-social-${p}`}
              />
            ))}
          </div>
        </FieldRow>

        {values.socialPlatform && (
          <FieldRow
            label={t("form.publicationMode")}
            error={errors.publicationMode}
            required
          >
            {values.socialPlatform === "threads" ? (
              <p className="text-sm text-gray-700">
                {t("form.publicationModeCreator")}
                <span className="ml-2 text-xs text-gray-500">
                  — {t("form.publicationModeThreadsHint")}
                </span>
              </p>
            ) : (
              <>
                <div className="flex flex-wrap gap-3">
                  {(
                    [
                      {
                        v: "creator" as const,
                        l: t("form.publicationModeCreator"),
                      },
                      {
                        v: "brand_only" as const,
                        l: t("form.publicationModeBrandOnly"),
                      },
                    ]
                  ).map(({ v, l }) => (
                    <RadioChip
                      key={v}
                      checked={values.publicationMode === v}
                      onChange={() => onChange("publicationMode", v)}
                      label={l}
                      testid={`form-publication-${v}`}
                    />
                  ))}
                </div>
                {values.publicationMode && (
                  <p className="mt-2 text-xs text-gray-500">
                    {values.publicationMode === "brand_only"
                      ? t("form.publicationModeBrandOnlyHint")
                      : t("form.publicationModeCreatorHint")}
                  </p>
                )}
              </>
            )}
          </FieldRow>
        )}

        {isInstagram && (
          <FieldRow
            label={t("form.contentFormatsInstagram")}
            error={errors.contentFormats}
            required
          >
            <div className="flex gap-3">
              {(["reels", "stories", "post"] as ContentFormat[]).map((f) => (
                <CheckboxChip
                  key={f}
                  checked={values.contentFormats.includes(f)}
                  onChange={() => toggleFormat(f)}
                  label={t(`format.${f}`)}
                  testid={`form-format-${f}`}
                />
              ))}
            </div>
          </FieldRow>
        )}

        {isInstagram && values.contentFormats.length > 0 && (
          <FieldRow
            label={t("form.postsByFormat")}
            error={errors.postsByFormat}
          >
            <div className="flex flex-wrap gap-3">
              {values.contentFormats.map((f) => (
                <div key={f} className="flex items-center gap-2">
                  <span className="text-sm text-gray-700">
                    {t(`format.${f}`)}
                  </span>
                  <PostCountInput
                    value={values.postsByFormat[f]}
                    onChange={(v) => {
                      const next = { ...values.postsByFormat };
                      if (v === undefined) delete next[f];
                      else next[f] = v;
                      onChange("postsByFormat", next);
                    }}
                    testid={`form-posts-${f}`}
                  />
                </div>
              ))}
            </div>
          </FieldRow>
        )}

        {values.socialPlatform &&
          values.socialPlatform !== "instagram" && (
          <FieldRow
            label={t("form.postsTotalLabel")}
            error={errors.postsByFormat}
            required
          >
            <PostCountInput
              value={values.postsByFormat.post}
              onChange={(v) => {
                const next: Partial<Record<ContentFormat, number>> = {};
                if (v !== undefined) next.post = v;
                onChange("postsByFormat", next);
              }}
              testid="form-posts-total"
            />
          </FieldRow>
        )}

        <FieldRow
          label={t("form.languages")}
          error={errors.languages}
          required
        >
          <div className="flex gap-3">
            {(["ru", "kz"] as Language[]).map((l) => (
              <CheckboxChip
                key={l}
                checked={values.languages.includes(l)}
                onChange={() =>
                  onChange("languages", toggleSet(values.languages, l))
                }
                label={t(`language.${l}`)}
                testid={`form-language-${l}`}
              />
            ))}
          </div>
        </FieldRow>

        <FieldRow
          label={t("form.publishDeadlineMode")}
          error={errors.publishDeadlineMode}
          required
        >
          <div className="flex flex-wrap gap-3">
            <RadioChip
              checked={values.publishDeadlineMode === "until"}
              onChange={() => onChange("publishDeadlineMode", "until")}
              label={t("form.publishDeadlineModeUntil")}
              testid="form-publish-deadline-mode-until"
            />
            <RadioChip
              checked={values.publishDeadlineMode === "exact"}
              onChange={() => onChange("publishDeadlineMode", "exact")}
              label={t("form.publishDeadlineModeExact")}
              testid="form-publish-deadline-mode-exact"
            />
          </div>
          {values.publishDeadlineMode && (
            <p className="mt-2 text-xs text-gray-500">
              {values.publishDeadlineMode === "exact"
                ? t("form.publishDeadlineModeExactHint")
                : t("form.publishDeadlineModeUntilHint")}
            </p>
          )}
        </FieldRow>

        <FieldRow
          label={
            values.publishDeadlineMode === "exact"
              ? t("form.publishDeadlineExactLabel")
              : values.publishDeadlineMode === "until"
                ? t("form.publishDeadlineUntilLabel")
                : t("form.publishDeadline")
          }
          error={errors.publishDeadline}
          required
        >
          <DatePicker
            value={values.publishDeadline}
            onChange={(v) => onChange("publishDeadline", v ?? "")}
            placeholder={t("form.publishDeadlinePlaceholder")}
            minDate={new Date()}
            testid="form-publish-deadline"
          />
        </FieldRow>

        {(values.socialPlatform === "instagram" ||
          values.socialPlatform === "tiktok") && (
          <FieldRow
            label={t("form.crossposting")}
            hint={t("form.crosspostingHint")}
            error={errors.crossposting}
            required
          >
            <div className="flex flex-wrap gap-3">
              <RadioChip
                checked={values.crossposting === "creator_choice"}
                onChange={() => onChange("crossposting", "creator_choice")}
                label={t("form.crosspostingCreatorChoice")}
                testid="form-crossposting-creator-choice"
              />
              {values.socialPlatform === "instagram" && (
                <RadioChip
                  checked={values.crossposting === "to_tiktok"}
                  onChange={() => onChange("crossposting", "to_tiktok")}
                  label={t("form.crosspostingToTiktok")}
                  testid="form-crossposting-to-tiktok"
                />
              )}
              {values.socialPlatform === "tiktok" && (
                <RadioChip
                  checked={values.crossposting === "to_instagram"}
                  onChange={() => onChange("crossposting", "to_instagram")}
                  label={t("form.crosspostingToInstagram")}
                  testid="form-crossposting-to-instagram"
                />
              )}
            </div>
          </FieldRow>
        )}
      </Section>

      <Section title={t("form.sections.brief")}>
        <FieldRow
          label={t("form.description")}
          error={errors.description}
          required
        >
          <textarea
            value={values.description}
            onChange={(e) => onChange("description", e.target.value)}
            placeholder={t("form.descriptionPlaceholder")}
            rows={4}
            className={inputCls}
            data-testid="form-description"
          />
        </FieldRow>

        <FieldRow
          label={t("form.references")}
          hint={t("form.referencesHint")}
        >
          <ReferencesField
            references={values.references}
            onChange={(refs) => onChange("references", refs)}
          />
        </FieldRow>

        <FieldRow
          label={t("form.hashtags")}
          hint={t("form.hashtagsHint")}
        >
          <input
            type="text"
            value={values.hashtags ?? ""}
            onChange={(e) => onChange("hashtags", e.target.value)}
            placeholder={t("form.hashtagsPlaceholder")}
            className={inputCls}
            data-testid="form-hashtags"
          />
        </FieldRow>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <FieldRow
            label={t("form.mentionsInCaption")}
            hint={t("form.mentionsInCaptionHint")}
          >
            <input
              type="text"
              value={values.mentionsInCaption ?? ""}
              onChange={(e) =>
                onChange("mentionsInCaption", e.target.value)
              }
              placeholder={t("form.mentionsInCaptionPlaceholder")}
              className={inputCls}
              data-testid="form-mentions-in-caption"
            />
          </FieldRow>
          <FieldRow
            label={t("form.mentionsInPublication")}
            hint={t("form.mentionsInPublicationHint")}
          >
            <input
              type="text"
              value={values.mentionsInPublication ?? ""}
              onChange={(e) =>
                onChange("mentionsInPublication", e.target.value)
              }
              placeholder={t("form.mentionsInPublicationPlaceholder")}
              className={inputCls}
              data-testid="form-mentions-in-publication"
            />
          </FieldRow>
        </div>

        <FieldRow label={t("form.adDisclaimer")}>
          <input
            type="text"
            value={values.adDisclaimer ?? ""}
            onChange={(e) => onChange("adDisclaimer", e.target.value)}
            placeholder={t("form.adDisclaimerPlaceholder")}
            className={inputCls}
            data-testid="form-ad-disclaimer"
          />
        </FieldRow>

        {values.socialPlatform === "instagram" &&
          values.publicationMode === "creator" && (
            <div>
              <CheckboxRow
                label={t("form.collabPost")}
                hint={t("form.collabPostHint")}
                checked={values.isCollabPost}
                onChange={(v) => {
                  onChange("isCollabPost", v);
                  if (!v) onChange("collabBrandHandles", []);
                  else if (values.collabBrandHandles.length === 0)
                    onChange("collabBrandHandles", [""]);
                }}
                testid="form-collab-post"
              />
              {values.isCollabPost && (
                <div className="ml-7 mt-2">
                  <FieldRow
                    label={t("form.collabBrandHandle")}
                    hint={t("form.collabHint")}
                    error={errors.collabBrandHandles}
                    required
                  >
                    <CollabHandlesField
                      handles={values.collabBrandHandles}
                      onChange={(next) => onChange("collabBrandHandles", next)}
                    />
                  </FieldRow>
                </div>
              )}
            </div>
          )}

        {values.adDisclaimer && values.adDisclaimer.trim() && (
          <FieldRow label={t("form.adDisclaimerPlacement")}>
            <div className="flex flex-wrap gap-3">
              {(
                [
                  {
                    v: "caption" as const,
                    l: t("form.adDisclaimerPlacementCaption"),
                  },
                  {
                    v: "publication" as const,
                    l: t("form.adDisclaimerPlacementPublication"),
                  },
                  {
                    v: "both" as const,
                    l: t("form.adDisclaimerPlacementBoth"),
                  },
                ]
              ).map(({ v, l }) => (
                <RadioChip
                  key={v}
                  checked={values.adDisclaimerPlacement === v}
                  onChange={() => onChange("adDisclaimerPlacement", v)}
                  label={l}
                  testid={`form-ad-disclaimer-${v}`}
                />
              ))}
            </div>
          </FieldRow>
        )}

        <FieldRow
          label={t("form.attachments")}
          hint={t("form.attachmentsHint")}
        >
          <AttachmentsField
            attachments={values.attachments}
            onChange={(items) => onChange("attachments", items)}
          />
        </FieldRow>
      </Section>

      <Section title={t("form.sections.approvals")}>
        <CheckboxRow
          label={t("form.scriptApproval")}
          hint={t("form.scriptApprovalHint")}
          checked={values.requiresScriptApproval}
          onChange={(v) => onChange("requiresScriptApproval", v)}
          testid="form-script-approval"
        />
        <CheckboxRow
          label={t("form.materialApproval")}
          hint={t("form.materialApprovalHint")}
          checked={values.requiresMaterialApproval}
          onChange={(v) => onChange("requiresMaterialApproval", v)}
          testid="form-material-approval"
        />
      </Section>

      <Section title={t("form.sections.payment")}>
        <FieldRow
          label={t("form.payment")}
          error={errors.paymentType}
          required
        >
          <div className="flex flex-wrap gap-3">
            {(
              [
                { v: "barter" as PaymentType, l: t("form.paymentBarter") },
                { v: "fixed" as PaymentType, l: t("form.paymentFixed") },
                {
                  v: "barter_fixed" as PaymentType,
                  l: t("form.paymentBarterFixed"),
                },
                {
                  v: "creator_proposal" as PaymentType,
                  l: t("form.paymentCreatorProposal"),
                },
              ]
            ).map(({ v, l }) => (
              <RadioChip
                key={v}
                checked={values.paymentType === v}
                onChange={() => onChange("paymentType", v)}
                label={l}
                testid={`form-payment-${v}`}
              />
            ))}
          </div>
        </FieldRow>

        {values.paymentType === "creator_proposal" && (
          <p className="mt-3 text-xs text-gray-500">
            {t("form.paymentCreatorProposalHint")}
          </p>
        )}

        {(values.paymentType === "fixed" ||
          values.paymentType === "barter_fixed") && (
          <FieldRow
            label={t("form.paymentAmount")}
            hint={t("form.paymentAmountHint")}
            error={errors.paymentAmount}
            required
          >
            <CurrencyInput
              value={values.paymentAmount}
              onChange={(v) => onChange("paymentAmount", v)}
              className="w-44"
              testid="form-payment-amount"
              ariaLabel={t("form.paymentAmount")}
            />
          </FieldRow>
        )}

        {(values.paymentType === "barter" ||
          values.paymentType === "barter_fixed") && (
          <>
            <FieldRow
              label={t("form.barterDescription")}
              error={errors.barterDescription}
              required
            >
              <textarea
                value={values.barterDescription ?? ""}
                onChange={(e) =>
                  onChange("barterDescription", e.target.value)
                }
                placeholder={t("form.barterDescriptionPlaceholder")}
                rows={3}
                className={inputCls}
                data-testid="form-barter-description"
              />
            </FieldRow>
            <FieldRow
              label={t("form.barterValue")}
              error={errors.barterValue}
              required
            >
              <CurrencyInput
                value={values.barterValue}
                onChange={(v) => onChange("barterValue", v)}
                className="w-44"
                testid="form-barter-value"
                ariaLabel={t("form.barterValue")}
              />
            </FieldRow>
            <FieldRow
              label={t("form.barterAttachments")}
              hint={t("form.barterAttachmentsHint")}
            >
              <AttachmentsField
                attachments={values.barterAttachments}
                onChange={(items) => onChange("barterAttachments", items)}
              />
            </FieldRow>
          </>
        )}
      </Section>
    </div>
  );
}

const inputCls =
  "w-full rounded-button border border-gray-300 bg-white px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100";

function Section({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-2xl border border-surface-200 bg-white p-6 shadow-sm">
      <h3 className="mb-5 border-b border-surface-200 pb-3 text-base font-semibold text-gray-900">
        {title}
      </h3>
      <div className="space-y-4">{children}</div>
    </section>
  );
}

function FieldRow({
  label,
  hint,
  error,
  required,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  required?: boolean;
  children: ReactNode;
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700">
        {label}
        {required && <span className="ml-1 text-red-500">*</span>}
      </label>
      {hint && <p className="text-xs text-gray-500">{hint}</p>}
      <div className="mt-1.5">{children}</div>
      {error && (
        <p className="mt-1 text-xs text-red-600" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function PostCountInput({
  value,
  onChange,
  testid,
}: {
  value?: number;
  onChange: (v: number | undefined) => void;
  testid: string;
}) {
  return (
    <div className="relative inline-block">
      <input
        type="number"
        min={1}
        value={value ?? ""}
        onChange={(e) =>
          onChange(e.target.value ? Number(e.target.value) : undefined)
        }
        className="w-20 rounded-button border border-gray-300 bg-white py-2 pl-3 pr-9 text-sm tabular-nums outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
        data-testid={testid}
      />
      <span
        className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-500"
        aria-hidden="true"
      >
        шт.
      </span>
    </div>
  );
}

function CheckboxChip({
  checked,
  onChange,
  label,
  testid,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
  testid: string;
}) {
  return (
    <button
      type="button"
      onClick={onChange}
      aria-pressed={checked}
      data-testid={testid}
      className={`rounded-full border px-3 py-1 text-sm font-medium transition ${
        checked
          ? "border-primary bg-primary text-white"
          : "border-surface-300 bg-white text-gray-700 hover:bg-surface-100"
      }`}
    >
      {label}
    </button>
  );
}

function RadioChip({
  checked,
  onChange,
  label,
  testid,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
  testid: string;
}) {
  return (
    <button
      type="button"
      onClick={onChange}
      aria-pressed={checked}
      data-testid={testid}
      className={`rounded-full border px-3 py-1 text-sm font-medium transition ${
        checked
          ? "border-primary bg-primary text-white"
          : "border-surface-300 bg-white text-gray-700 hover:bg-surface-100"
      }`}
    >
      {label}
    </button>
  );
}

function CheckboxRow({
  label,
  hint,
  checked,
  onChange,
  testid,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  testid: string;
}) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="mt-0.5 h-4 w-4 rounded border-gray-300 accent-primary text-primary focus:ring-primary"
        data-testid={testid}
      />
      <span>
        <span className="text-sm font-medium text-gray-700">{label}</span>
        {hint && <span className="block text-xs text-gray-500">{hint}</span>}
      </span>
    </label>
  );
}

function CollabHandlesField({
  handles,
  onChange,
}: {
  handles: string[];
  onChange: (next: string[]) => void;
}) {
  const { t } = useTranslation("campaigns");
  function update(idx: number, v: string) {
    const next = [...handles];
    next[idx] = v;
    onChange(next);
  }
  function remove(idx: number) {
    onChange(handles.filter((_, i) => i !== idx));
  }
  function add() {
    onChange([...handles, ""]);
  }
  return (
    <div className="space-y-2">
      {handles.map((h, i) => (
        <div key={i} className="flex items-center gap-2">
          <input
            type="text"
            value={h}
            onChange={(e) => update(i, e.target.value)}
            placeholder={t("form.collabBrandHandlePlaceholder")}
            className={`${inputCls} max-w-sm`}
            data-testid={`form-collab-brand-handle-${i}`}
          />
          {handles.length > 1 && (
            <button
              type="button"
              onClick={() => remove(i)}
              className="text-xs text-red-600 hover:underline"
              aria-label="Удалить бренд"
            >
              ✕
            </button>
          )}
        </div>
      ))}
      <button
        type="button"
        onClick={add}
        className="rounded-button border border-dashed border-surface-300 px-3 py-1.5 text-xs text-gray-600 hover:bg-surface-100"
        data-testid="form-collab-add-brand"
      >
        + {t("form.collabAddBrand")}
      </button>
    </div>
  );
}

function ReferencesField({
  references,
  onChange,
}: {
  references: Reference[];
  onChange: (refs: Reference[]) => void;
}) {
  const { t } = useTranslation("campaigns");
  function update(idx: number, patch: Partial<Reference>) {
    const next = [...references];
    const cur = next[idx];
    if (!cur) return;
    next[idx] = { ...cur, ...patch };
    onChange(next);
  }
  function remove(idx: number) {
    onChange(references.filter((_, i) => i !== idx));
  }
  function add() {
    onChange([...references, { url: "", description: "" }]);
  }
  return (
    <div className="space-y-2">
      {references.map((ref, i) => (
        <div
          key={i}
          className="rounded-card border border-surface-200 bg-surface-100 p-3"
        >
          <input
            type="url"
            value={ref.url}
            onChange={(e) => update(i, { url: e.target.value })}
            placeholder={t("form.referenceUrl")}
            className={inputCls}
            data-testid={`form-reference-url-${i}`}
          />
          <input
            type="text"
            value={ref.description}
            onChange={(e) => update(i, { description: e.target.value })}
            placeholder={t("form.referenceDescription")}
            className={`${inputCls} mt-2`}
            data-testid={`form-reference-desc-${i}`}
          />
          <button
            type="button"
            onClick={() => remove(i)}
            className="mt-2 text-xs text-red-600 hover:underline"
          >
            {t("form.removeReference")}
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={add}
        className="rounded-button border border-dashed border-surface-300 px-3 py-2 text-sm text-gray-600 hover:bg-surface-100"
        data-testid="form-add-reference"
      >
        + {t("form.addReference")}
      </button>
    </div>
  );
}

function AttachmentsField({
  attachments,
  onChange,
}: {
  attachments: Attachment[];
  onChange: (items: Attachment[]) => void;
}) {
  const { t } = useTranslation("campaigns");
  function handleFiles(e: ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
    if (!files) return;
    const additions: Attachment[] = Array.from(files).map((f) => ({
      id: `att-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name: f.name,
      url: `/mock-files/${f.name}`,
      contentType: f.type || "application/octet-stream",
      sizeBytes: f.size,
    }));
    onChange([...attachments, ...additions]);
    e.target.value = "";
  }
  function remove(id: string) {
    onChange(attachments.filter((a) => a.id !== id));
  }
  return (
    <div className="space-y-2">
      {attachments.map((a) => (
        <div
          key={a.id}
          className="flex items-center gap-3 rounded-button border border-surface-200 bg-surface-100 px-3 py-2"
        >
          <span className="flex-1 truncate text-sm text-gray-900">
            {a.name}
          </span>
          <span className="text-xs text-gray-500">
            {(a.sizeBytes / 1024).toFixed(1)} КБ
          </span>
          <button
            type="button"
            onClick={() => remove(a.id)}
            className="text-xs text-red-600 hover:underline"
          >
            ✕
          </button>
        </div>
      ))}
      <label className="inline-block cursor-pointer rounded-button border border-dashed border-surface-300 px-3 py-2 text-sm text-gray-600 hover:bg-surface-100">
        + {t("form.addFile")}
        <input
          type="file"
          multiple
          onChange={handleFiles}
          className="hidden"
          data-testid="form-attachments-input"
        />
      </label>
    </div>
  );
}

function EventDetailsFields({
  requiresVisit,
  value,
  onChange,
  error,
}: {
  requiresVisit: boolean;
  value: EventDetails;
  onChange: (v: EventDetails) => void;
  error?: string;
}) {
  const { t } = useTranslation("campaigns");
  const citiesQuery = useQuery({
    queryKey: dictionaryKeys.list("cities"),
    queryFn: () => listDictionary("cities"),
    staleTime: 5 * 60 * 1000,
  });
  const cityOptions = citiesQuery.data ?? [];
  function patch(p: Partial<EventDetails>) {
    onChange({ ...value, ...p });
  }
  function setTransfer(next: EventTransfer | undefined) {
    if (next === undefined) {
      const { transfer: _drop, ...rest } = value;
      void _drop;
      onChange(rest as EventDetails);
    } else {
      patch({ transfer: next });
    }
  }
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <FieldRow label={t("form.eventCountry")} required>
          <Select
            options={COUNTRIES}
            value={findCountryCode(value.country)}
            onChange={(code) =>
              patch({
                country:
                  COUNTRIES.find((c) => c.code === code)?.name ?? "",
              })
            }
            placeholder={t("form.eventCountryPlaceholder")}
            searchable={false}
            testid="form-event-country"
          />
        </FieldRow>
        <FieldRow label={t("form.eventCity")} required>
          <Select
            options={cityOptions}
            value={cityOptions.find((c) => c.name === value.city)?.code}
            onChange={(code) =>
              patch({
                city:
                  cityOptions.find((c) => c.code === code)?.name ?? "",
              })
            }
            placeholder={t("form.eventCity")}
            isLoading={citiesQuery.isLoading}
            testid="form-event-city"
          />
        </FieldRow>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <FieldRow label={t("form.eventDate")} required>
          <DatePicker
            value={value.date}
            onChange={(v) => patch({ date: v ?? "" })}
            placeholder={t("form.eventDate")}
            minDate={new Date()}
            testid="form-event-date"
          />
        </FieldRow>
        <FieldRow label={t("form.eventTimeFrom")} required>
          <div className="flex items-center gap-2">
            <input
              type="time"
              value={value.timeFrom}
              onChange={(e) => patch({ timeFrom: e.target.value })}
              className={`${inputCls} w-28`}
              data-testid="form-event-time-from"
            />
            <span className="text-sm text-gray-500">
              {t("form.eventTimeTo")}
            </span>
            <input
              type="time"
              value={value.timeTo}
              onChange={(e) => patch({ timeTo: e.target.value })}
              className={`${inputCls} w-28`}
              data-testid="form-event-time-to"
            />
          </div>
        </FieldRow>
      </div>

      <FieldRow label={t("form.eventAddress")} required>
        <input
          type="text"
          value={value.address}
          onChange={(e) => patch({ address: e.target.value })}
          placeholder={t("form.eventAddressPlaceholder")}
          className={inputCls}
          data-testid="form-event-address"
        />
      </FieldRow>

      {requiresVisit && (
        <>
          <FieldRow label={t("form.eventDressCode")}>
            <input
              type="text"
              value={value.dressCode ?? ""}
              onChange={(e) => patch({ dressCode: e.target.value })}
              placeholder={t("form.eventDressCodePlaceholder")}
              className={inputCls}
              data-testid="form-event-dress-code"
            />
          </FieldRow>

          <FieldRow label={t("form.eventParking")}>
            <div className="flex flex-wrap gap-3">
              {(["paid", "free", "none"] as EventParking[]).map((p) => (
                <RadioChip
                  key={p}
                  checked={value.parking === p}
                  onChange={() => patch({ parking: p })}
                  label={t(
                    p === "paid"
                      ? "form.eventParkingPaid"
                      : p === "free"
                        ? "form.eventParkingFree"
                        : "form.eventParkingNone",
                  )}
                  testid={`form-event-parking-${p}`}
                />
              ))}
            </div>
          </FieldRow>

          <FieldRow label={t("form.eventEntryInstructions")}>
            <textarea
              value={value.entryInstructions ?? ""}
              onChange={(e) => patch({ entryInstructions: e.target.value })}
              placeholder={t("form.eventEntryInstructionsPlaceholder")}
              rows={3}
              className={inputCls}
              data-testid="form-event-entry-instructions"
            />
          </FieldRow>

          <FieldRow label={t("form.eventTransfer")}>
            <div className="flex gap-3">
              <RadioChip
                checked={!!value.transfer}
                onChange={() =>
                  setTransfer(value.transfer ?? { type: "personal" })
                }
                label={t("form.eventTransferYes")}
                testid="form-event-transfer-yes"
              />
              <RadioChip
                checked={!value.transfer}
                onChange={() => setTransfer(undefined)}
                label={t("form.eventTransferNo")}
                testid="form-event-transfer-no"
              />
            </div>
          </FieldRow>

          {value.transfer && (
            <div className="space-y-3 rounded-card border border-surface-200 bg-surface-100 p-3">
              <FieldRow label={t("form.eventTransferType")}>
                <div className="flex gap-3">
                  {(["personal", "group"] as EventTransferType[]).map((tp) => (
                    <RadioChip
                      key={tp}
                      checked={value.transfer?.type === tp}
                      onChange={() =>
                        setTransfer({ ...value.transfer!, type: tp })
                      }
                      label={t(
                        tp === "personal"
                          ? "form.eventTransferPersonal"
                          : "form.eventTransferGroup",
                      )}
                      testid={`form-event-transfer-type-${tp}`}
                    />
                  ))}
                </div>
              </FieldRow>

              {value.transfer.type === "group" && (
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <FieldRow label={t("form.eventTransferPickup")} required>
                    <input
                      type="text"
                      value={value.transfer.pickup ?? ""}
                      onChange={(e) =>
                        setTransfer({
                          ...value.transfer!,
                          pickup: e.target.value,
                        })
                      }
                      placeholder={t("form.eventTransferPickupPlaceholder")}
                      className={inputCls}
                      data-testid="form-event-transfer-pickup"
                    />
                  </FieldRow>
                  <FieldRow label={t("form.eventTransferSchedule")} required>
                    <input
                      type="text"
                      value={value.transfer.schedule ?? ""}
                      onChange={(e) =>
                        setTransfer({
                          ...value.transfer!,
                          schedule: e.target.value,
                        })
                      }
                      placeholder={t("form.eventTransferSchedulePlaceholder")}
                      className={inputCls}
                      data-testid="form-event-transfer-schedule"
                    />
                  </FieldRow>
                </div>
              )}
            </div>
          )}
        </>
      )}

      {error && (
        <p className="text-xs text-red-600" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function VisitDetailsField({
  visit,
  onChange,
}: {
  visit: VisitDetails;
  onChange: (v: VisitDetails) => void;
}) {
  const { t } = useTranslation("campaigns");

  function addSlot() {
    onChange({
      ...visit,
      slots: [...visit.slots, { date: "", time: "" }],
    });
  }
  function removeSlot(idx: number) {
    onChange({
      ...visit,
      slots: visit.slots.filter((_, i) => i !== idx),
    });
  }
  function updateSlot(idx: number, patch: { date?: string; time?: string }) {
    const next = [...visit.slots];
    const cur = next[idx];
    if (!cur) return;
    next[idx] = { ...cur, ...patch };
    onChange({ ...visit, slots: next });
  }

  return (
    <div className="rounded-card border border-surface-200 bg-surface-100 p-3">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <div>
          <label className="block text-xs text-gray-500">
            {t("form.visitCity")}
          </label>
          <input
            type="text"
            value={visit.city}
            onChange={(e) => onChange({ ...visit, city: e.target.value })}
            className={inputCls}
            data-testid="form-visit-city"
          />
        </div>
        <div>
          <label className="block text-xs text-gray-500">
            {t("form.visitAddress")}
          </label>
          <input
            type="text"
            value={visit.address}
            onChange={(e) => onChange({ ...visit, address: e.target.value })}
            className={inputCls}
            data-testid="form-visit-address"
          />
        </div>
      </div>

      <div className="mt-3">
        <p className="text-xs text-gray-500">
          {t("form.visitTime")} —{" "}
          {visit.slots.length === 0 ? t("form.visitAnyTime") : t("form.visitSlots")}
        </p>
        <div className="mt-2 space-y-2">
          {visit.slots.map((s, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                type="date"
                value={s.date}
                onChange={(e) => updateSlot(i, { date: e.target.value })}
                className={`${inputCls} w-44`}
              />
              <input
                type="time"
                value={s.time}
                onChange={(e) => updateSlot(i, { time: e.target.value })}
                className={`${inputCls} w-32`}
              />
              <button
                type="button"
                onClick={() => removeSlot(i)}
                className="text-xs text-red-600 hover:underline"
              >
                ✕
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addSlot}
            className="text-xs font-medium text-primary hover:underline"
            data-testid="form-add-slot"
          >
            + {t("form.addSlot")}
          </button>
        </div>
      </div>
    </div>
  );
}
