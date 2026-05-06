// Returns the currently selected list-row id, prioritising the live URL
// over the React-state mirror.
//
// Why: when keyboard handlers in a list-with-drawer (Arrow keys) fire
// faster than React can commit the re-render between presses, the handler
// closure sees the previous render's `selectedId` and the second press
// no-ops. In the browser, setSearchParams updates `window.location`
// synchronously via history.pushState, so the URL is always fresh. In
// jsdom + MemoryRouter that path doesn't run, so we fall back to a ref
// mirror updated in the render body.
import type { RefObject } from "react";

export function readCurrentId(
  selectedIdRef: RefObject<string | null>,
): string | null {
  const fromUrl = new URL(window.location.href).searchParams.get("id");
  return fromUrl ?? selectedIdRef.current ?? null;
}
