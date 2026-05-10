import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import type { components } from "@/api/generated/schema";
import Spinner from "@/shared/components/Spinner";
import CategoryChip from "@/shared/components/CategoryChip";
import { calcAge } from "@/shared/utils/age";
import SocialAdminRow from "./SocialAdminRow";
import VerifyManualDialog from "./VerifyManualDialog";
import type { ApplicationDetail } from "../types";
import { DrawerContext } from "./drawerContext";

type DetailSocial = components["schemas"]["CreatorApplicationDetailSocial"];

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
  footer?: ReactNode;
}

export default function ApplicationDrawer(props: ApplicationDrawerProps) {
  if (!props.open) return null;
  return <OpenDrawer {...props} />;
}

function OpenDrawer({
  application,
  isLoading,
  isError,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
  footer,
}: ApplicationDrawerProps) {
  const { t } = useTranslation("creatorApplications");
  const dialogRef = useRef<HTMLDivElement>(null);
  const [apiError, setApiError] = useState("");

  useEffect(() => {
    dialogRef.current?.focus();

    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft" && canPrev && onPrev) onPrev();
      if (e.key === "ArrowRight" && canNext && onNext) onNext();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [onClose, onPrev, onNext, canPrev, canNext]);

  const ctx = useMemo(
    () => ({ onApiError: setApiError, onCloseDrawer: onClose }),
    [onClose],
  );

  return (
    <DrawerContext.Provider value={ctx}>
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
            {apiError && (
              <div
                className="mb-3 rounded-button border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
                role="alert"
                data-testid="drawer-api-error"
              >
                {apiError}
              </div>
            )}
            {isLoading ? (
              <Spinner className="mt-4" />
            ) : isError || !application ? (
              <p className="text-gray-500" data-testid="drawer-error">
                {t("loadError")}
              </p>
            ) : (
              <ApplicationBody
                application={application}
                onCloseDrawer={onClose}
                onApiError={setApiError}
              />
            )}
          </div>

          {footer && (
            <footer
              className="shrink-0 border-t border-surface-300 bg-white px-6 py-3"
              data-testid="drawer-footer"
            >
              {footer}
            </footer>
          )}
        </div>
      </div>
    </DrawerContext.Provider>
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

function ApplicationBody({
  application,
  onCloseDrawer,
  onApiError,
}: {
  application: ApplicationDetail;
  onCloseDrawer: () => void;
  onApiError: (message: string) => void;
}) {
  const { t } = useTranslation("creatorApplications");
  const age = calcAge(application.birthDate);
  const tg = application.telegramLink;
  const telegramLinked = !!tg;
  const [verifyTarget, setVerifyTarget] = useState<DetailSocial | null>(null);

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

        <UtmSection application={application} />

        <Field
          fullWidth
          label={t("drawer.socials")}
          value={
            <div className="flex flex-col" data-testid="drawer-socials">
              {application.socials.map((s) => (
                <SocialAdminRow
                  key={s.id}
                  social={s}
                  telegramLinked={telegramLinked}
                  onVerifyClick={(target) => {
                    onApiError("");
                    setVerifyTarget(target);
                  }}
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

      {verifyTarget && (
        <VerifyManualDialog
          open
          applicationId={application.id}
          social={verifyTarget}
          onClose={() => setVerifyTarget(null)}
          onCloseDrawer={onCloseDrawer}
          onApiError={onApiError}
        />
      )}
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

const UTM_KEYS = ["source", "medium", "campaign", "term", "content"] as const;
type UtmKey = (typeof UTM_KEYS)[number];

function UtmSection({ application }: { application: ApplicationDetail }) {
  const { t } = useTranslation("creatorApplications");
  const values: Record<UtmKey, string | null | undefined> = {
    source: application.utmSource,
    medium: application.utmMedium,
    campaign: application.utmCampaign,
    term: application.utmTerm,
    content: application.utmContent,
  };
  const present = UTM_KEYS.filter((key) => {
    const v = values[key];
    return typeof v === "string" && v.length > 0;
  });
  if (present.length === 0) return null;

  return (
    <Field
      fullWidth
      testid="utm-section"
      label={t("drawer.utm.title")}
      value={
        <div className="flex flex-col gap-0.5">
          {present.map((key) => (
            <span key={key} className="text-sm text-gray-900">
              <span className="text-gray-500">{t(`drawer.utm.${key}`)}:</span>{" "}
              <span data-testid={`utm-${key}-value`}>{values[key]}</span>
            </span>
          ))}
        </div>
      }
    />
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
