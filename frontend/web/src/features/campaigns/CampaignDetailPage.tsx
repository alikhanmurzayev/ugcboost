import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { getCampaign, type Campaign } from "@/api/campaigns";
import { getCreator } from "@/api/creators";
import { ApiError } from "@/api/client";
import { ROUTES, SEARCH_PARAMS } from "@/shared/constants/routes";
import { campaignKeys, creatorKeys } from "@/shared/constants/queryKeys";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import CreatorDrawer from "@/features/creators/CreatorDrawer";
import CampaignEditSection from "./CampaignEditSection";
import CampaignCreatorsSection from "./creators/CampaignCreatorsSection";
import { useCampaignCreators } from "./creators/hooks/useCampaignCreators";

const SAFE_LINK_SCHEMES = new Set(["http:", "https:", "tg:"]);

export default function CampaignDetailPage() {
  const { t } = useTranslation("campaigns");
  const { campaignId } = useParams<{ campaignId: string }>();
  const id = campaignId ?? "";

  const detailQuery = useQuery({
    queryKey: campaignKeys.detail(id),
    queryFn: () => getCampaign(id),
    enabled: !!id,
    retry: false,
  });

  if (detailQuery.isLoading) {
    return (
      <div data-testid="campaign-detail-page">
        <Spinner className="mt-12" />
      </div>
    );
  }

  if (detailQuery.isError) {
    const err = detailQuery.error;
    if (err instanceof ApiError && err.status === 404) {
      return <NotFoundState />;
    }
    return (
      <div data-testid="campaign-detail-page">
        <ErrorState
          message={t("detail.loadError")}
          onRetry={() => void detailQuery.refetch()}
        />
      </div>
    );
  }

  const campaign = detailQuery.data?.data;
  if (!campaign) return <NotFoundState />;

  return <CampaignDetailContent campaign={campaign} />;
}

function NotFoundState() {
  const { t } = useTranslation("campaigns");
  return (
    <div data-testid="campaign-detail-page">
      <div data-testid="campaign-detail-not-found" className="max-w-2xl">
        <Link
          to={`/${ROUTES.CAMPAIGNS}`}
          className="text-sm text-gray-500 hover:text-gray-700"
          data-testid="campaign-detail-back"
        >
          {t("detail.backToList")}
        </Link>
        <h1 className="mt-2 text-2xl font-bold text-gray-900">
          {t("detail.notFoundTitle")}
        </h1>
        <p className="mt-1 text-sm text-gray-500">
          {t("detail.notFoundMessage")}
        </p>
      </div>
    </div>
  );
}

function CampaignDetailContent({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("campaigns");
  const [isEditing, setIsEditing] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();

  const selectedCreatorId = searchParams.get(SEARCH_PARAMS.CREATOR_ID);

  // Reuses the same React Query key as <CampaignCreatorsSection>; both
  // subscriptions share one network round-trip via TanStack's request
  // dedup, so the prefill comes for free on cold-load with ?creatorId=X.
  const { rows } = useCampaignCreators(campaign.id, {
    enabled: !campaign.isDeleted,
  });

  const detailQuery = useQuery({
    queryKey: creatorKeys.detail(selectedCreatorId ?? ""),
    queryFn: () => getCreator(selectedCreatorId ?? ""),
    enabled: !!selectedCreatorId,
    retry: false,
  });

  const prefill = selectedCreatorId
    ? rows.find((r) => r.campaignCreator.creatorId === selectedCreatorId)
        ?.creator
    : undefined;

  function closeCreator() {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev);
      np.delete(SEARCH_PARAMS.CREATOR_ID);
      return np;
    });
  }

  return (
    <div data-testid="campaign-detail-page" className="max-w-7xl">
      <div className="max-w-2xl">
        <Link
          to={`/${ROUTES.CAMPAIGNS}`}
          className="text-sm text-gray-500 hover:text-gray-700"
          data-testid="campaign-detail-back"
        >
          {t("detail.backToList")}
        </Link>
        <div className="mt-2 flex items-center gap-3">
          <h1
            className="text-2xl font-bold text-gray-900"
            data-testid="campaign-detail-title"
          >
            {campaign.name}
          </h1>
          {campaign.isDeleted && (
            <span
              className="inline-flex items-center rounded-full bg-surface-200 px-2 py-0.5 text-xs font-medium text-gray-500"
              data-testid="campaign-detail-deleted-badge"
            >
              {t("labels.deletedBadge")}
            </span>
          )}
        </div>

        <section
          className="mt-6 rounded-card border border-surface-300 bg-white p-6"
          data-testid="campaign-section-about"
        >
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-bold text-gray-900">
              {isEditing ? t("edit.title") : t("detail.sectionTitle")}
            </h2>
            {!isEditing && (
              <button
                type="button"
                onClick={() => setIsEditing(true)}
                disabled={campaign.isDeleted}
                title={
                  campaign.isDeleted
                    ? t("detail.editDisabledHint")
                    : undefined
                }
                className="rounded-button border border-surface-300 px-3 py-1.5 text-sm text-gray-700 transition hover:bg-surface-200 disabled:cursor-not-allowed disabled:opacity-50"
                data-testid="campaign-edit-button"
              >
                {t("detail.editButton")}
              </button>
            )}
          </div>

          {isEditing ? (
            <CampaignEditSection
              campaign={campaign}
              onCancel={() => setIsEditing(false)}
              onSaved={() => setIsEditing(false)}
            />
          ) : (
            <ViewSection campaign={campaign} />
          )}
        </section>
      </div>

      <CampaignCreatorsSection campaign={campaign} />

      <CreatorDrawer
        prefill={prefill}
        detail={detailQuery.data?.data}
        isLoading={detailQuery.isLoading}
        isError={detailQuery.isError}
        open={!!selectedCreatorId}
        onClose={closeCreator}
      />
    </div>
  );
}

function ViewSection({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("campaigns");
  const safeUrl = safeHref(campaign.tmaUrl);
  return (
    <dl className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
      <Field label={t("detail.nameLabel")}>
        <span data-testid="campaign-detail-name">{campaign.name}</span>
      </Field>
      <Field label={t("detail.tmaUrlLabel")}>
        {safeUrl ? (
          <a
            href={safeUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="break-all text-primary hover:underline"
            data-testid="campaign-detail-tma-url"
          >
            {campaign.tmaUrl}
          </a>
        ) : (
          <span
            className="break-all text-gray-700"
            data-testid="campaign-detail-tma-url"
          >
            {campaign.tmaUrl}
          </span>
        )}
      </Field>
      <Field label={t("detail.createdAtLabel")}>
        <span data-testid="campaign-detail-created-at">
          {formatDateTime(campaign.createdAt)}
        </span>
      </Field>
      <Field label={t("detail.updatedAtLabel")}>
        <span data-testid="campaign-detail-updated-at">
          {formatDateTime(campaign.updatedAt)}
        </span>
      </Field>
    </dl>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd className="mt-1 text-sm text-gray-900">{children}</dd>
    </div>
  );
}

// safeHref returns the original URL only if it parses and uses an allowed
// scheme (http / https / tg). Backend campaign validation does not enforce a
// scheme, so a stored `javascript:`/`data:` URL would otherwise execute on
// click — defence-in-depth from the frontend side.
function safeHref(raw: string): string | null {
  try {
    const u = new URL(raw);
    return SAFE_LINK_SCHEMES.has(u.protocol) ? raw : null;
  } catch {
    return null;
  }
}

function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString("ru", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
