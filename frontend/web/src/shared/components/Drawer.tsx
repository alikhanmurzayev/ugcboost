import { useEffect, useRef, type ReactNode } from "react";

interface DrawerProps {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  children: ReactNode;
  widthClassName?: string;
}

export default function Drawer({
  open,
  onClose,
  title,
  children,
  widthClassName = "w-[480px]",
}: DrawerProps) {
  const dialogRef = useRef<HTMLElement>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Focus the dialog only on open transitions. Re-running on every onClose
  // identity change steals focus from descendants (e.g. the search input)
  // because parents typically pass an inline arrow / fresh closure.
  useEffect(() => {
    if (!open) return;
    dialogRef.current?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onCloseRef.current();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open]);

  if (!open) return null;

  return (
    <>
      <button
        type="button"
        aria-label="Закрыть панель"
        onClick={onClose}
        data-testid="drawer-backdrop"
        className="fixed inset-0 z-40 cursor-default bg-black/30 focus:outline-none"
      />
      <aside
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        className={`fixed inset-y-0 right-0 z-50 ${widthClassName} max-w-full overflow-y-auto bg-white shadow-xl outline-none`}
        data-testid="drawer"
      >
        <header className="flex items-center justify-between border-b border-surface-300 px-6 py-4">
          <div
            className="text-lg font-semibold text-gray-900"
            data-testid="drawer-title"
          >
            {title}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Закрыть"
            className="rounded-button p-1 text-gray-500 hover:bg-surface-200 hover:text-gray-900"
            data-testid="drawer-close"
          >
            <svg
              className="h-5 w-5"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </header>
        <div className="px-6 py-5">{children}</div>
      </aside>
    </>
  );
}
