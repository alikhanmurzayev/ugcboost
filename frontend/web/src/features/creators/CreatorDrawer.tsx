import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Spinner from "@/shared/components/Spinner";
import CreatorDrawerBody from "./CreatorDrawerBody";
import type { CreatorListItem, CreatorAggregate } from "./types";

interface CreatorDrawerProps {
  prefill: CreatorListItem | undefined;
  detail: CreatorAggregate | undefined;
  isLoading?: boolean;
  isError?: boolean;
  open: boolean;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  canPrev?: boolean;
  canNext?: boolean;
}

export default function CreatorDrawer(props: CreatorDrawerProps) {
  if (!props.open) return null;
  return <OpenDrawer {...props} />;
}

function OpenDrawer({
  prefill,
  detail,
  isLoading,
  isError,
  onClose,
  onPrev,
  onNext,
  canPrev,
  canNext,
}: CreatorDrawerProps) {
  const { t } = useTranslation("creators");
  const dialogRef = useRef<HTMLDivElement>(null);

  // Focus dialog once on mount; do not steal focus on every prop change.
  useEffect(() => {
    dialogRef.current?.focus();
  }, []);

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft" && canPrev && onPrev) onPrev();
      if (e.key === "ArrowRight" && canNext && onNext) onNext();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [onClose, onPrev, onNext, canPrev, canNext]);

  const fullName = buildFullName(detail, prefill);
  const showBody = !!(detail || prefill);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        aria-label={t("drawer.closeWindow")}
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
          title={fullName}
          onClose={onClose}
          onPrev={onPrev}
          onNext={onNext}
          canPrev={canPrev}
          canNext={canNext}
          prevLabel={t("drawerNav.prev")}
          nextLabel={t("drawerNav.next")}
        />

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {isError && !detail ? (
            <p className="text-gray-500" data-testid="drawer-error">
              {t("loadError")}
            </p>
          ) : (
            <>
              {showBody && (
                <CreatorDrawerBody prefill={prefill} detail={detail} />
              )}
              {isLoading && !detail && (
                <div data-testid="drawer-detail-spinner">
                  <Spinner className="mt-4" />
                </div>
              )}
            </>
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
  const { t } = useTranslation("creators");
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
        aria-label={t("drawer.close")}
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

function buildFullName(
  detail: CreatorAggregate | undefined,
  prefill: CreatorListItem | undefined,
): string {
  if (detail) {
    return [detail.lastName, detail.firstName, detail.middleName]
      .filter(Boolean)
      .join(" ");
  }
  if (prefill) {
    return [prefill.lastName, prefill.firstName, prefill.middleName]
      .filter(Boolean)
      .join(" ");
  }
  return "";
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
