import { useEffect } from "react";
import type { CreatorReel } from "../types";

interface Props {
  reel: CreatorReel;
  onClose: () => void;
}

export default function ReelLightbox({ reel, onClose }: Props) {
  useEffect(() => {
    function handle(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-gray-950/90 p-4"
      onClick={onClose}
      data-testid="reel-lightbox"
    >
      <button
        type="button"
        onClick={onClose}
        aria-label="Закрыть"
        className="absolute right-4 top-4 rounded-full bg-white/10 p-2 text-white hover:bg-white/20"
        data-testid="reel-lightbox-close"
      >
        <svg
          width="22"
          height="22"
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
      <video
        key={reel.id}
        src={reel.videoUrl}
        poster={reel.thumbnailUrl}
        controls
        autoPlay
        playsInline
        loop
        onClick={(e) => e.stopPropagation()}
        className="max-h-[92vh] max-w-full rounded-2xl bg-black object-contain shadow-2xl"
      />
    </div>
  );
}
