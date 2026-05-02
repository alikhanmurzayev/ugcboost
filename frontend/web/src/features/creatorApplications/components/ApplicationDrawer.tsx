import { useEffect, useRef, useState, type ReactNode } from "react";
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
      <div
        className="flex-1 truncate px-2 text-lg font-semibold text-gray-900"
        data-testid="drawer-full-name"
      >
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
          testid="drawer-birth-date"
          label={t("drawer.birthDate")}
          value={`${formatDate(application.birthDate)} · ${age} ${pluralYears(age)}`}
        />
        <Field
          testid="drawer-iin"
          label={t("drawer.iin")}
          value={application.iin}
        />
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
        <Field
          testid="drawer-city"
          label={t("drawer.city")}
          value={application.city.name}
        />

        <Field
          fullWidth
          label={t("drawer.categories")}
          value={
            <div className="flex flex-wrap gap-1.5">
              {application.categories.map((c) => (
                <CategoryChip
                  key={c.code}
                  testid={`drawer-category-${c.code}`}
                >
                  {c.name}
                </CategoryChip>
              ))}
              {application.categoryOtherText && (
                <CategoryChip testid="drawer-category-other-text">
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
              <div data-testid="drawer-telegram-not-linked">
                <span className="text-sm text-gray-400">
                  {t("drawer.telegramNotLinked")}
                </span>
                <CopyBotMessageButton
                  message={t("drawer.botMessageTemplate", {
                    url: application.telegramBotUrl,
                  })}
                  copyLabel={t("drawer.copyBotMessage")}
                  copiedLabel={t("drawer.botMessageCopied")}
                  hint={t("drawer.botMessageHint")}
                />
              </div>
            )
          }
        />
      </dl>
    </>
  );
}

interface CopyBotMessageButtonProps {
  message: string;
  copyLabel: string;
  copiedLabel: string;
  hint: string;
}

function CopyBotMessageButton({
  message,
  copyLabel,
  copiedLabel,
  hint,
}: CopyBotMessageButtonProps) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(message);
      setCopied(true);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard может быть недоступен (старый браузер / insecure context).
      // В этом случае ничего не делаем — UI просто не покажет confirm,
      // оператор увидит что не сработало и попросит у нас починить.
    }
  }

  return (
    <div className="mt-2 flex flex-col gap-1.5">
      <button
        type="button"
        onClick={handleCopy}
        data-testid="drawer-copy-bot-message"
        className={`inline-flex w-fit items-center gap-1.5 rounded-button border px-3 py-1.5 text-sm font-medium transition ${
          copied
            ? "border-emerald-300 bg-emerald-50 text-emerald-700"
            : "border-surface-300 bg-white text-gray-700 hover:bg-surface-100"
        }`}
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
        {copied ? copiedLabel : copyLabel}
      </button>
      <p className="text-xs text-gray-500">{hint}</p>
    </div>
  );
}

function CopyIcon() {
  return (
    <svg
      className="h-4 w-4"
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
      className="h-4 w-4"
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
