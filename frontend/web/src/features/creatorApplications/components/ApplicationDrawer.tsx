import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Spinner from "@/shared/components/Spinner";
import SocialLink from "./SocialLink";
import CategoryChip from "./CategoryChip";
import { calcAge } from "../filters";
import type { ApplicationDetail } from "../types";

interface ApplicationDrawerProps {
  application: ApplicationDetail | undefined;
  isLoading?: boolean;
  isError?: boolean;
  open: boolean;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  canPrev?: boolean;
  canNext?: boolean;
}

export default function ApplicationDrawer({
  application,
  isLoading,
  isError,
  open,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
}: ApplicationDrawerProps) {
  const { t } = useTranslation("creatorApplications");
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
          ) : isError || !application ? (
            <p className="text-gray-500" data-testid="drawer-error">
              {t("loadError")}
            </p>
          ) : (
            <ApplicationBody application={application} />
          )}
        </div>
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

function ApplicationBody({ application }: { application: ApplicationDetail }) {
  const { t } = useTranslation("creatorApplications");
  const age = calcAge(application.birthDate);
  const tg = application.telegramLink;

  return (
    <>
      <div
        className="space-y-0.5 text-xs text-gray-500"
        data-testid="application-timeline"
      >
        <p>
          {t("drawer.submittedAt")}: {formatDateTime(application.createdAt)}
        </p>
      </div>

      <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4">
        <Field
          label={t("drawer.birthDate")}
          value={`${formatDate(application.birthDate)} · ${age} ${pluralYears(age)}`}
        />
        <Field label={t("drawer.iin")} value={application.iin} />
        <Field
          label={t("drawer.phone")}
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
        <Field label={t("drawer.city")} value={application.city.name} />

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

        <Field
          fullWidth
          label={t("drawer.telegram")}
          value={
            tg ? (
              <span className="text-sm text-gray-900" data-testid="drawer-telegram-linked">
                {tg.telegramUsername
                  ? `@${tg.telegramUsername}`
                  : [tg.telegramFirstName, tg.telegramLastName]
                      .filter(Boolean)
                      .join(" ") || `id ${tg.telegramUserId}`}
              </span>
            ) : (
              <span
                className="text-sm text-gray-400"
                data-testid="drawer-telegram-not-linked"
              >
                {t("drawer.telegramNotLinked")}
              </span>
            )
          }
        />
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

function buildFullName(application: ApplicationDetail): string {
  return [application.lastName, application.firstName, application.middleName]
    .filter(Boolean)
    .join(" ");
}

function pluralYears(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "год";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return "года";
  return "лет";
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
