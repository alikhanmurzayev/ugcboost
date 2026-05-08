/**
 * Shared UI-level e2e helpers for the web admin app.
 *
 * Anything that drives the browser through Playwright (clicks, fills,
 * navigations) and is reused across more than one spec file lives here.
 * Pure HTTP helpers (seeding, cleanup, raw API calls) belong in `./api.ts`.
 */
import { expect, type Page } from "@playwright/test";

// loginAs walks the admin login form and waits for the post-login redirect
// to the dashboard. Test data-testids are owned by `LoginPage`. Used by
// admin-side specs that need an authenticated browser context.
export async function loginAs(
  page: Page,
  email: string,
  password: string,
): Promise<void> {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByTestId("email-input").fill(email);
  await page.getByTestId("password-input").fill(password);
  await page.getByTestId("login-button").click();
  await expect(page).toHaveURL("/");
}
