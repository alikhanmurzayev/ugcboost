import { defineConfig } from "@playwright/test";

const BASE_URL = process.env.BASE_URL || "http://localhost:4321";

export default defineConfig({
  testDir: "../e2e/landing",
  timeout: 30_000,
  retries: 0,
  reporter: [["html", { open: "never" }]],
  use: {
    baseURL: BASE_URL,
    headless: true,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    // On staging, both the landing host and staging-api sit behind CF Access.
    // Forwarding the service-token headers on every request (page navigation +
    // XHR from the page) lets Playwright punch through Access without a user
    // session. Mirrors frontend/web/playwright.config.ts.
    extraHTTPHeaders: process.env.CF_ACCESS_CLIENT_ID
      ? {
          "CF-Access-Client-Id": process.env.CF_ACCESS_CLIENT_ID,
          "CF-Access-Client-Secret": process.env.CF_ACCESS_CLIENT_SECRET ?? "",
        }
      : undefined,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
  // CI=true → both backend and landing are already up via docker-compose
  // (Makefile target test-e2e-landing). Local `npx playwright test` boots
  // its own stack: backend on :8080 (go run) and Astro dev on :4321. Same
  // shape as frontend/web/playwright.config.ts.
  // Astro reads window.__RUNTIME_CONFIG__ from public/config.js — the dev
  // server serves the local fallback file (apiUrl http://localhost:8080),
  // which matches the webServer backend port below.
  webServer: process.env.CI
    ? undefined
    : [
        {
          command:
            "cd ../../backend && ENVIRONMENT=local DATABASE_URL='postgres://ugcboost:ugcboost_dev@localhost:5433/ugcboost?sslmode=disable' JWT_SECRET='test-secret' ADMIN_PASSWORD='admin123' JWT_EXPIRY='15m' CORS_ORIGINS='http://localhost:4321' PORT=8080 TELEGRAM_BOT_USERNAME='ugcboost_e2e_bot' LEGAL_AGREEMENT_VERSION='2026-04-20' LEGAL_PRIVACY_VERSION='2026-04-20' go run ./cmd/api/",
          port: 8080,
          reuseExistingServer: true,
          timeout: 30_000,
        },
        {
          command: "npx astro dev --port 4321",
          port: 4321,
          reuseExistingServer: true,
          timeout: 15_000,
        },
      ],
});
