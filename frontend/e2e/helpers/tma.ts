/**
 * TMA-specific browser helpers. The TMA frontend boots
 * @telegram-apps/sdk-react which reads launch params from the URL hash
 * fragment (`#tgWebAppData=...&tgWebAppVersion=...`). For Playwright tests
 * we either:
 *   1. Inject a fully-formed signed initData into the hash before the SDK
 *      runs, OR
 *   2. Stub `window.Telegram.WebApp` directly and skip the SDK auto-read.
 *
 * Approach #1 is closer to production wiring and works without a single
 * `window.Telegram.WebApp` shim, so this helper goes with that path.
 */
import type { Page } from "@playwright/test";

// mockTelegramWebApp injects a signed initData query string into the page
// URL hash before any JS runs, mimicking how a real Telegram client opens
// a TMA. The SDK's `retrieveRawInitData()` then returns this exact string,
// which the TMA frontend forwards via `Authorization: tma <init-data>`.
export async function mockTelegramWebApp(page: Page, initData: string) {
  await page.addInitScript((data: string) => {
    const hash =
      "#tgWebAppPlatform=web&tgWebAppVersion=8.0&tgWebAppData=" +
      encodeURIComponent(data);
    try {
      window.location.hash = hash;
    } catch {
      // no-op: addInitScript runs in every frame, including about:blank
      // before location.hash is writable. Subsequent navigations get the
      // hash via the same script.
    }
  }, initData);
}
