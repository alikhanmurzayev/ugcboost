import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { createCampaign, getCampaign, updateCampaign } from "@/_prototype/api/campaigns";
import { listDictionary } from "@/_prototype/api/dictionaries";
import { campaignKeys, dictionaryKeys } from "@/_prototype/queryKeys";
import { ROUTES } from "@/_prototype/routes";
import { ApiError } from "@/_prototype/api/client";
import { getErrorMessage } from "@/_prototype/shared/i18n/errors";
import Spinner from "@/_prototype/shared/components/Spinner";
import CampaignTypeStep from "./components/CampaignTypeStep";
import CampaignForm, {
  type CampaignFormValues,
} from "./components/CampaignForm";
import type { Campaign, CampaignType } from "./types";

const INITIAL_VALUES: CampaignFormValues = {
  title: "",
  description: "",
  references: [],
  attachments: [],
  creatorsCount: 0,
  anyCategories: false,
  categoryCodes: [],
  minFollowers: 0,
  minAvgViews: 0,
  anyCities: false,
  cityCodes: [],
  genders: [],
  languages: [],
  contentFormats: [],
  postsByFormat: {},
  publishDeadline: "",
  barterAttachments: [],
  isCollabPost: false,
  collabBrandHandles: [],
  requiresScriptApproval: false,
  requiresMaterialApproval: false,
  requiresVisit: false,
};

export default function CampaignNewPage() {
  const { t } = useTranslation("prototype_campaigns");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { campaignId } = useParams<{ campaignId: string }>();
  const isEdit = !!campaignId;

  const editQuery = useQuery({
    queryKey: campaignKeys.detail(campaignId ?? ""),
    queryFn: () => getCampaign(campaignId ?? ""),
    enabled: isEdit,
  });

  const [type, setType] = useState<CampaignType | null>(null);
  const [values, setValues] = useState<CampaignFormValues>(INITIAL_VALUES);
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => {
    if (!isEdit || !editQuery.data || hydrated) return;
    const c = editQuery.data;
    setType(c.type);
    setValues(campaignToFormValues(c));
    setHydrated(true);
  }, [isEdit, editQuery.data, hydrated]);
  const [errors, setErrors] = useState<
    Partial<Record<keyof CampaignFormValues, string>>
  >({});
  const [topError, setTopError] = useState("");

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

  function update<K extends keyof CampaignFormValues>(
    key: K,
    value: CampaignFormValues[K],
  ) {
    setValues((prev) => ({ ...prev, [key]: value }));
    if (errors[key]) {
      setErrors((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
    }
  }

  const mutation = useMutation({
    mutationFn: ({
      asDraft,
      campaignType,
    }: {
      asDraft: boolean;
      campaignType: CampaignType;
    }) => {
      // Validate already enforced these to be non-empty for non-draft submissions;
      // for drafts we fall back to safe defaults so the API contract holds.
      const socialPlatform = values.socialPlatform ?? "instagram";
      const publishDeadlineMode = values.publishDeadlineMode ?? "until";
      const paymentType = values.paymentType ?? "barter";
      const publicationMode =
        socialPlatform === "threads"
          ? "creator"
          : (values.publicationMode ?? "creator");
      const crossposting =
        socialPlatform === "threads"
          ? "creator_choice"
          : (values.crossposting ?? "creator_choice");

      const cities = values.cityCodes
        .map((code) =>
          (citiesQuery.data ?? []).find((c) => c.code === code),
        )
        .filter((c): c is { code: string; name: string; sortOrder: number } => !!c);
      const categories = values.categoryCodes
        .map((code) =>
          (categoriesQuery.data ?? []).find((c) => c.code === code),
        )
        .filter((c): c is { code: string; name: string; sortOrder: number } => !!c);

      const publishDeadline = values.publishDeadline
        ? new Date(values.publishDeadline + "T23:59:59").toISOString()
        : new Date().toISOString();

      const isInstagram = socialPlatform === "instagram";
      const formats = isInstagram ? values.contentFormats : ["post" as const];
      const postsByFormat = isInstagram
        ? values.postsByFormat
        : { post: values.postsByFormat.post };

      const payload = {
          type: campaignType,
          title: values.title.trim(),
          description: values.description.trim(),
          hashtags: values.hashtags?.trim() || undefined,
          mentionsInCaption: values.mentionsInCaption?.trim() || undefined,
          mentionsInPublication:
            values.mentionsInPublication?.trim() || undefined,
          adDisclaimer: values.adDisclaimer?.trim() || undefined,
          adDisclaimerPlacement: values.adDisclaimer?.trim()
            ? values.adDisclaimerPlacement
            : undefined,
          references: values.references.filter((r) => r.url.trim()),
          attachments: values.attachments,
          creatorsCount: values.creatorsCount,
          anyCategories: values.anyCategories,
          categories: values.anyCategories
            ? []
            : categories.map(({ code, name }) => ({ code, name })),
          minFollowers: values.minFollowers,
          minAvgViews: values.minAvgViews,
          ageMin: values.ageMin,
          ageMax: values.ageMax,
          anyCities: values.anyCities,
          cities: values.anyCities
            ? []
            : cities.map(({ code, name }) => ({ code, name })),
          genders: values.genders,
          languages: values.languages,
          socialPlatform,
          contentFormats: formats,
          postsByFormat,
          publishDeadline,
          publishDeadlineMode,
          paymentType,
          paymentAmount:
            paymentType === "fixed" || paymentType === "barter_fixed"
              ? values.paymentAmount
              : undefined,
          barterDescription:
            paymentType === "barter" || paymentType === "barter_fixed"
              ? values.barterDescription?.trim()
              : undefined,
          barterValue:
            paymentType === "barter" || paymentType === "barter_fixed"
              ? values.barterValue
              : undefined,
          barterAttachments:
            paymentType === "barter" || paymentType === "barter_fixed"
              ? values.barterAttachments
              : [],
          publicationMode,
          isCollabPost:
            socialPlatform === "instagram" &&
            publicationMode === "creator" &&
            values.isCollabPost,
          collabBrandHandles:
            socialPlatform === "instagram" &&
            publicationMode === "creator" &&
            values.isCollabPost
              ? values.collabBrandHandles
                  .map((h) => h.trim().replace(/^@+/, ""))
                  .filter((h) => h.length > 0)
              : [],
          crossposting,
          requiresScriptApproval: values.requiresScriptApproval,
          requiresMaterialApproval: values.requiresMaterialApproval,
          requiresVisit: values.requiresVisit,
          visitDetails:
            campaignType === "event" ? undefined : values.visitDetails,
          eventDetails:
            campaignType === "event" ? values.eventDetails : undefined,
          halal: values.halal,
        };
      return isEdit && campaignId
        ? updateCampaign(campaignId, payload, asDraft)
        : createCampaign(payload, asDraft);
    },
    onSuccess: (campaign, variables) => {
      queryClient.invalidateQueries({ queryKey: campaignKeys.all() });
      const target = variables.asDraft
        ? ROUTES.CAMPAIGNS_DRAFT
        : ROUTES.CAMPAIGNS_PENDING;
      navigate("/prototype/" + target);
      void campaign;
    },
    onError(err) {
      setTopError(
        err instanceof ApiError ? getErrorMessage(err.code) : "Не удалось сохранить",
      );
    },
  });

  function validate(): boolean {
    const e: Partial<Record<keyof CampaignFormValues, string>> = {};
    if (!values.title.trim()) e.title = t("validation.titleRequired");
    if (!values.description.trim())
      e.description = t("validation.descriptionRequired");
    if (!values.creatorsCount || values.creatorsCount < 1)
      e.creatorsCount = t("validation.creatorsCountRequired");
    if (!values.anyCategories && values.categoryCodes.length === 0)
      e.categoryCodes = t("validation.categoriesRequired");
    if (!values.anyCities && values.cityCodes.length === 0)
      e.cityCodes = t("validation.citiesRequired");
    if (values.genders.length === 0) e.genders = t("validation.gendersRequired");
    if (values.languages.length === 0)
      e.languages = t("validation.languagesRequired");
    if (!values.socialPlatform)
      e.socialPlatform = t("validation.socialPlatformRequired");
    if (!values.publishDeadlineMode)
      e.publishDeadlineMode = t("validation.publishDeadlineModeRequired");
    if (!values.paymentType)
      e.paymentType = t("validation.paymentTypeRequired");
    if (
      values.socialPlatform &&
      values.socialPlatform !== "threads" &&
      !values.publicationMode
    )
      e.publicationMode = t("validation.publicationModeRequired");
    if (values.socialPlatform && values.socialPlatform !== "threads" && !values.crossposting)
      e.crossposting = t("validation.crosspostingRequired");
    const isInstagram = values.socialPlatform === "instagram";
    if (isInstagram) {
      if (values.contentFormats.length === 0)
        e.contentFormats = t("validation.contentFormatsRequired");
      else if (
        values.contentFormats.some(
          (f) => !values.postsByFormat[f] || values.postsByFormat[f]! < 1,
        )
      ) {
        e.postsByFormat = t("validation.postsCountRequired");
      }
    } else if (
      !values.postsByFormat.post ||
      values.postsByFormat.post < 1
    ) {
      e.postsByFormat = t("validation.postsCountRequired");
    }
    if (!values.publishDeadline)
      e.publishDeadline = t("validation.publishDeadlineRequired");

    if (
      (values.paymentType === "fixed" ||
        values.paymentType === "barter_fixed") &&
      (!values.paymentAmount || values.paymentAmount <= 0)
    )
      e.paymentAmount = t("validation.paymentAmountRequired");
    if (
      values.paymentType === "barter" ||
      values.paymentType === "barter_fixed"
    ) {
      if (!values.barterDescription?.trim())
        e.barterDescription = t("validation.barterDescriptionRequired");
      if (!values.barterValue || values.barterValue <= 0)
        e.barterValue = t("validation.barterValueRequired");
    }

    if (type === "event") {
      const ev = values.eventDetails;
      if (!ev?.country.trim())
        e.eventDetails = t("validation.eventCountryRequired");
      else if (!ev.city.trim())
        e.eventDetails = t("validation.eventCityRequired");
      else if (!ev.date) e.eventDetails = t("validation.eventDateRequired");
      else if (!ev.timeFrom || !ev.timeTo)
        e.eventDetails = t("validation.eventTimeRequired");
      else if (!ev.address.trim())
        e.eventDetails = t("validation.eventAddressRequired");
      else if (
        values.requiresVisit &&
        ev.transfer?.type === "group" &&
        (!ev.transfer.pickup?.trim() || !ev.transfer.schedule?.trim())
      )
        e.eventDetails = t("validation.eventTransferPickupRequired");
    }
    if (type === "food" && values.halal === undefined)
      e.halal = t("validation.halalRequired");
    if (
      values.socialPlatform === "instagram" &&
      values.publicationMode === "creator" &&
      values.isCollabPost &&
      values.collabBrandHandles.every((h) => !h.trim())
    )
      e.collabBrandHandles = t("validation.collabBrandHandleRequired");
    if (
      type !== "event" &&
      values.requiresVisit &&
      (!values.visitDetails?.city.trim() || !values.visitDetails?.address.trim())
    ) {
      e.visitDetails = t("validation.visitAddressRequired");
    }
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  function handleSubmit() {
    if (!type) return;
    setTopError("");
    if (!validate()) return;
    mutation.mutate({ asDraft: false, campaignType: type });
  }

  function handleSaveDraft() {
    if (!type) return;
    setTopError("");
    setErrors({});
    mutation.mutate({ asDraft: true, campaignType: type });
  }

  if (isEdit && (!editQuery.data || !hydrated)) {
    return (
      <div data-testid="campaign-new-page">
        <Spinner className="mt-12" />
      </div>
    );
  }

  if (!type) {
    return (
      <div data-testid="campaign-new-page">
        <Link
          to={"/prototype/" + ROUTES.CAMPAIGNS_ACTIVE}
          className="text-sm text-primary hover:underline"
        >
          ← {t("backToList")}
        </Link>
        <h1 className="mt-3 text-2xl font-bold text-gray-900">
          {t("wizard.title")}
        </h1>
        <div className="mt-6">
          <CampaignTypeStep onSelect={setType} />
        </div>
      </div>
    );
  }

  return (
    <div data-testid="campaign-new-page" className="max-w-3xl">
      {!isEdit && (
        <button
          type="button"
          onClick={() => setType(null)}
          className="text-sm text-primary hover:underline"
          data-testid="campaign-form-back"
        >
          ← {t("wizard.back")}
        </button>
      )}
      {isEdit && (
        <Link
          to={"/prototype/" + ROUTES.CAMPAIGN_DETAIL(campaignId!)}
          className="text-sm text-primary hover:underline"
        >
          ← {t("backToList")}
        </Link>
      )}
      <h1 className="mt-3 flex flex-wrap items-baseline gap-3 text-2xl font-bold text-gray-900">
        {isEdit ? t("wizard.editTitle") : t("wizard.title")}
        <span className="text-base font-medium text-gray-500">
          · {t(`types.${type}`)}
        </span>
      </h1>
      {!isEdit && (
        <p className="text-sm text-gray-500">{t("wizard.stepForm")}</p>
      )}

      <div className="mt-6">
        <CampaignForm
          type={type}
          values={values}
          errors={errors}
          onChange={update}
        />
      </div>

      {topError && (
        <p className="mt-6 text-sm text-red-600" role="alert">
          {topError}
        </p>
      )}

      <div className="mt-8 flex flex-wrap items-center justify-end gap-3 border-t border-surface-300 pt-6">
        <button
          type="button"
          onClick={handleSaveDraft}
          disabled={mutation.isPending}
          className="rounded-button border border-surface-300 bg-white px-4 py-2 text-sm font-semibold text-gray-700 hover:bg-surface-100 disabled:opacity-50"
          data-testid="campaign-save-draft"
        >
          {mutation.isPending && mutation.variables?.asDraft
            ? t("wizard.saving")
            : t("wizard.saveDraft")}
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={mutation.isPending}
          className="rounded-button bg-primary px-4 py-2 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
          data-testid="campaign-submit"
        >
          {mutation.isPending && !mutation.variables?.asDraft
            ? t("wizard.submitting")
            : t("wizard.submit")}
        </button>
      </div>
    </div>
  );
}

function campaignToFormValues(c: Campaign): CampaignFormValues {
  // Convert ISO datetime back to YYYY-MM-DD for the date picker.
  const deadline = c.publishDeadline ? c.publishDeadline.slice(0, 10) : "";
  return {
    title: c.title,
    description: c.description,
    hashtags: c.hashtags,
    mentionsInCaption: c.mentionsInCaption,
    mentionsInPublication: c.mentionsInPublication,
    adDisclaimer: c.adDisclaimer,
    adDisclaimerPlacement: c.adDisclaimerPlacement,
    references: c.references,
    attachments: c.attachments,
    creatorsCount: c.creatorsCount,
    anyCategories: c.anyCategories,
    categoryCodes: c.categories.map((cat) => cat.code),
    minFollowers: c.minFollowers,
    minAvgViews: c.minAvgViews,
    ageMin: c.ageMin,
    ageMax: c.ageMax,
    anyCities: c.anyCities,
    cityCodes: c.cities.map((ct) => ct.code),
    genders: c.genders,
    languages: c.languages,
    socialPlatform: c.socialPlatform,
    contentFormats: c.contentFormats,
    postsByFormat: c.postsByFormat,
    publishDeadline: deadline,
    publishDeadlineMode: c.publishDeadlineMode,
    paymentType: c.paymentType,
    paymentAmount: c.paymentAmount,
    barterDescription: c.barterDescription,
    barterValue: c.barterValue,
    barterAttachments: c.barterAttachments,
    publicationMode: c.publicationMode,
    isCollabPost: c.isCollabPost,
    collabBrandHandles: c.collabBrandHandles,
    crossposting: c.crossposting,
    requiresScriptApproval: c.requiresScriptApproval,
    requiresMaterialApproval: c.requiresMaterialApproval,
    requiresVisit: c.requiresVisit,
    visitDetails: c.visitDetails,
    eventDetails: c.eventDetails,
    halal: c.halal,
  };
}
