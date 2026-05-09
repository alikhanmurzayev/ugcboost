import { retrieveRawInitData } from "@telegram-apps/sdk";
import type { Middleware } from "openapi-fetch";

// readInitDataFromHash mirrors what @telegram-apps/sdk does internally for
// `tgWebAppData`. The SDK requires a successful `init()` call early in app
// boot, but in our flow we just want the raw payload — so we keep a thin
// hash reader as a fallback when the SDK throws (e.g., test harness that
// injects the hash via addInitScript before SDK init runs).
function readInitDataFromHash(): string | undefined {
  if (typeof window === "undefined") return undefined;
  const raw = window.location.hash.startsWith("#")
    ? window.location.hash.slice(1)
    : window.location.hash;
  if (!raw) return undefined;
  const params = new URLSearchParams(raw);
  const initData = params.get("tgWebAppData");
  return initData ?? undefined;
}

export const tmaInitDataMiddleware: Middleware = {
  onRequest({ request }) {
    let initData: string | undefined;
    try {
      initData = retrieveRawInitData();
    } catch {
      initData = readInitDataFromHash();
    }
    if (!initData) {
      initData = readInitDataFromHash();
    }
    if (initData) {
      request.headers.set("Authorization", `tma ${initData}`);
    }
    return request;
  },
};
