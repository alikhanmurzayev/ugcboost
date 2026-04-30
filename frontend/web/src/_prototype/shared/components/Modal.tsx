import { useEffect, useRef, type ReactNode } from "react";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  widthClassName?: string;
}

export default function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  widthClassName = "w-[28rem]",
}: ModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    dialogRef.current?.focus();

    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Закрыть окно"
        onClick={onClose}
        data-testid="modal-backdrop"
        className="absolute inset-0 cursor-default bg-black/30 focus:outline-none"
      />
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        className={`relative z-10 ${widthClassName} max-w-full rounded-card bg-white shadow-xl outline-none`}
        data-testid="modal"
      >
        <header className="border-b border-surface-300 px-6 py-4">
          <div className="text-lg font-semibold text-gray-900">{title}</div>
        </header>
        <div className="px-6 py-5">{children}</div>
        {footer && (
          <footer className="flex justify-end gap-2 border-t border-surface-300 px-6 py-4">
            {footer}
          </footer>
        )}
      </div>
    </div>
  );
}
