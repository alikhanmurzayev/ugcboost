import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Spinner from "@/shared/components/Spinner";
import SocialLink from "./SocialLink";
import QualityIndicatorDot from "./QualityIndicatorDot";
import CategoryChip from "./CategoryChip";
import type { Application } from "../types";

interface ApplicationDrawerProps {
  application: Application | undefined;
  isLoading?: boolean;
  open: boolean;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  canPrev?: boolean;
  canNext?: boolean;
  children?: ReactNode;
}

export default function ApplicationDrawer({
  application,
  isLoading,
  open,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
  children,
}: ApplicationDrawerProps) {
  const { t } = useTranslation("prototype_creatorApplications");
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    dialogRef.current?.focus();

    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft" && canPrev && onPrev) onPrev();
      if (e.key === "ArrowRight" && canNext && onNext) onNext();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open, onClose, onPrev, onNext, canPrev, canNext]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Закрыть окно"
        onClick={onClose}
        data-testid="drawer-backdrop"
        className="absolute inset-0 cursor-default bg-black/30 focus:outline-none"
      />
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        className="relative z-10 flex max-h-[90vh] w-[680px] max-w-full flex-col overflow-hidden rounded-card bg-white shadow-xl outline-none"
        data-testid="drawer"
      >
        <DrawerHeader
          title={application ? buildFullName(application) : ""}
          onClose={onClose}
          onPrev={onPrev}
          onNext={onNext}
          canPrev={canPrev}
          canNext={canNext}
          prevLabel={t("drawerNav.prev")}
          nextLabel={t("drawerNav.next")}
        />

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {isLoading ? (
            <Spinner className="mt-4" />
          ) : !application ? (
            <p className="text-gray-500">{t("loadError")}</p>
          ) : (
            <ApplicationBody application={application} />
          )}
        </div>

        {children && application && (
          <footer
            className="shrink-0 border-t border-surface-300 bg-white px-6 py-3"
            data-testid="drawer-footer"
          >
            {children}
          </footer>
        )}
      </div>
    </div>
  );
}

interface DrawerHeaderProps {
  title: string;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  canPrev?: boolean;
  canNext?: boolean;
  prevLabel: string;
  nextLabel: string;
}

function DrawerHeader({
  title,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
  prevLabel,
  nextLabel,
}: DrawerHeaderProps) {
  const showNav = !!(onPrev || onNext);
  return (
    <header className="flex shrink-0 items-center gap-2 border-b border-surface-300 px-4 py-3">
      {showNav && (
        <>
          <NavButton
            onClick={onPrev}
            disabled={!canPrev}
            label={prevLabel}
            testid="drawer-prev"
          >
            <ChevronLeft />
          </NavButton>
          <NavButton
            onClick={onNext}
            disabled={!canNext}
            label={nextLabel}
            testid="drawer-next"
          >
            <ChevronRight />
          </NavButton>
        </>
      )}
      <div className="flex-1 truncate px-2 text-lg font-semibold text-gray-900">
        {title}
      </div>
      <button
        type="button"
        onClick={onClose}
        aria-label="Закрыть"
        className="rounded-button p-1.5 text-gray-500 hover:bg-surface-200 hover:text-gray-900"
        data-testid="drawer-close"
      >
        <CrossIcon />
      </button>
    </header>
  );
}

function NavButton({
  onClick,
  disabled,
  label,
  testid,
  children,
}: {
  onClick?: () => void;
  disabled?: boolean;
  label: string;
  testid: string;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
      data-testid={testid}
      className="rounded-button p-1.5 text-gray-500 hover:bg-surface-200 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-gray-500"
    >
      {children}
    </button>
  );
}

function ApplicationBody({ application }: { application: Application }) {
  const { t } = useTranslation(["prototype_creatorApplications", "prototype_creators"]);
  const age = calcAge(application.birthDate);

  return (
    <>
      <div
        className="space-y-0.5 text-xs text-gray-500"
        data-testid="application-timeline"
      >
        <p>
          {t("drawer.submittedAt")}: {formatDateTime(application.createdAt)}
        </p>
        {application.approvedAt && (
          <p>
            {t("drawer.approvedAt")}: {formatDateTime(application.approvedAt)}
          </p>
        )}
        {application.signedAt && (
          <p>
            {t("drawer.signedAt")}: {formatDateTime(application.signedAt)}
          </p>
        )}
        {application.rejectedAt && (
          <p>
            {t("drawer.rejectedAt")}: {formatDateTime(application.rejectedAt)}
          </p>
        )}
      </div>

      <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4">
        <Field
          label={t("prototype_creatorApplications:drawer.birthDate")}
          value={`${formatDate(application.birthDate)} · ${age} ${pluralYears(age)}`}
        />
        <Field label={t("prototype_creatorApplications:drawer.iin")} value={application.iin} />
        <Field
          label={t("prototype_creatorApplications:drawer.phone")}
          value={
            <a
              href={`tel:${application.phone}`}
              className="text-primary hover:underline"
              data-testid="application-phone"
            >
              {application.phone}
            </a>
          }
        />
        <Field label={t("prototype_creatorApplications:drawer.city")} value={application.city.name} />

        {application.qualityIndicator && (
          <Field
            fullWidth
            label={t("qualityIndicator.label")}
            value={
              <div>
                <div className="flex items-center gap-3">
                  <QualityIndicatorDot
                    value={application.qualityIndicator}
                    size="md"
                    withLabel
                  />
                  <img
                    src="/social/instagram.svg"
                    alt="Instagram"
                    title={t("qualityIndicator.source")}
                    className="h-4 w-4 opacity-70"
                  />
                </div>
                {application.metrics && (
                  <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-sm">
                    <dt className="text-gray-500">{t("metrics.followers")}</dt>
                    <dd>
                      <MetricValue tone={followerTone(application.metrics.followers)}>
                        {formatNumber(application.metrics.followers)}
                      </MetricValue>
                    </dd>
                    <dt className="text-gray-500">{t("metrics.totalPosts")}</dt>
                    <dd>
                      <MetricValue tone={postsTone(application.metrics.totalPosts)}>
                        {formatNumber(application.metrics.totalPosts)}
                      </MetricValue>
                    </dd>
                    <dt className="text-gray-500">{t("metrics.avgViews")}</dt>
                    <dd>
                      <MetricValue tone={viewsTone(application.metrics.avgViews)}>
                        {formatNumber(application.metrics.avgViews)}
                      </MetricValue>
                    </dd>
                    <dt className="text-gray-500">{t("metrics.engagementRate")}</dt>
                    <dd>
                      <MetricValue tone={erTone(application.metrics.engagementRate)}>
                        {application.metrics.engagementRate.toFixed(1)}%
                      </MetricValue>
                    </dd>
                    <dt className="text-gray-500">
                      {t("metrics.postedLast14Days")}
                    </dt>
                    <dd>
                      {application.metrics.postedLast14Days ? (
                        <MetricValue tone="green">✓ {t("metrics.yes")}</MetricValue>
                      ) : (
                        <MetricValue tone="red">✗ {t("metrics.no")}</MetricValue>
                      )}
                    </dd>
                  </dl>
                )}
              </div>
            }
          />
        )}

        <Field
          fullWidth
          label={t("drawer.categories")}
          value={
            <div className="flex flex-wrap gap-1.5">
              {application.categories.map((c) => (
                <CategoryChip key={c.code}>{c.name}</CategoryChip>
              ))}
              {application.categoryOtherText && (
                <CategoryChip>
                  <span className="italic">
                    {t("drawer.categoryOther")}: {application.categoryOtherText}
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
            <div className="flex flex-wrap gap-x-5 gap-y-1.5">
              {application.socials.map((s) => (
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

        {application.stage === "contracts" && (
          <Field
            fullWidth
            label={t("drawer.contractStatus")}
            value={<ContractStatusBadge status={application.contractStatus} />}
          />
        )}

        {application.stage === "creators" && (
          <div className="col-span-2 grid grid-cols-3 gap-x-6 gap-y-4 border-t border-surface-200 pt-4">
            <Field
              label={t("prototype_creators:drawer.rating")}
              value={<StarRating value={application.rating} />}
            />
            <Field
              label={t("prototype_creators:drawer.completedOrders")}
              value={
                application.completedOrders !== undefined
                  ? application.completedOrders
                  : "—"
              }
            />
            <Field
              label={t("prototype_creators:drawer.activeOrders")}
              value={
                application.activeOrders !== undefined
                  ? application.activeOrders
                  : "—"
              }
            />
          </div>
        )}

        {application.stage === "rejected" && application.rejectionComment && (
          <Field
            fullWidth
            label={t("drawer.rejectionComment")}
            value={
              <p className="rounded-card bg-surface-100 p-3 text-sm whitespace-pre-wrap">
                {application.rejectionComment}
              </p>
            }
          />
        )}
      </dl>
    </>
  );
}

function Field({
  label,
  value,
  fullWidth = false,
}: {
  label: string;
  value: ReactNode;
  fullWidth?: boolean;
}) {
  return (
    <div className={fullWidth ? "col-span-2" : ""}>
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd className="mt-1 text-sm text-gray-900">{value}</dd>
    </div>
  );
}

function ContractStatusBadge({
  status,
}: {
  status?: "not_sent" | "sent" | "signed";
}) {
  const { t } = useTranslation("prototype_creatorApplications");
  const variant =
    status === "signed"
      ? "bg-emerald-100 text-emerald-800"
      : status === "sent"
        ? "bg-sky-100 text-sky-800"
        : "bg-amber-100 text-amber-800";
  const key = status ?? "not_sent";
  return (
    <span
      className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${variant}`}
      data-testid="contract-status-badge"
    >
      {t(`contractStatus.${key}`)}
    </span>
  );
}

function StarRating({ value }: { value?: number }) {
  const { t } = useTranslation("prototype_creators");
  if (value === undefined) {
    return <span className="text-sm text-gray-400">{t("drawer.noRatingYet")}</span>;
  }
  return (
    <span className="inline-flex items-center gap-1">
      <StarIcon />
      <span className="font-medium">{value.toFixed(1)}</span>
      <span className="text-gray-400">/ 5</span>
    </span>
  );
}

function StarIcon() {
  return (
    <svg
      className="h-4 w-4 text-amber-500"
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
    </svg>
  );
}

function ChevronLeft() {
  return (
    <svg
      className="h-5 w-5"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="15 18 9 12 15 6" />
    </svg>
  );
}

function ChevronRight() {
  return (
    <svg
      className="h-5 w-5"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="9 18 15 12 9 6" />
    </svg>
  );
}

function CrossIcon() {
  return (
    <svg
      className="h-5 w-5"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function buildFullName(application: Application): string {
  return [application.lastName, application.firstName, application.middleName]
    .filter(Boolean)
    .join(" ");
}

function calcAge(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}

function pluralYears(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "год";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return "года";
  return "лет";
}

type MetricTone = "green" | "orange" | "red";

const METRIC_TONE_CLASS: Record<MetricTone, string> = {
  green: "text-emerald-700",
  orange: "text-amber-700",
  red: "text-red-700",
};

function MetricValue({
  tone,
  children,
}: {
  tone: MetricTone;
  children: ReactNode;
}) {
  return (
    <span className={`font-medium ${METRIC_TONE_CLASS[tone]}`}>{children}</span>
  );
}

function followerTone(n: number): MetricTone {
  if (n >= 1000) return "green";
  if (n >= 500) return "orange";
  return "red";
}

function postsTone(n: number): MetricTone {
  if (n >= 100) return "green";
  if (n >= 50) return "orange";
  return "red";
}

function viewsTone(n: number): MetricTone {
  if (n >= 3000) return "green";
  if (n >= 1000) return "orange";
  return "red";
}

function erTone(percent: number): MetricTone {
  if (percent >= 5) return "green";
  if (percent >= 2) return "orange";
  return "red";
}

function formatNumber(n: number): string {
  return n.toLocaleString("ru");
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
