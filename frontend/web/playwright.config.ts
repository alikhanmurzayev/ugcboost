import { defineConfig } from "@playwright/test";

const BASE_URL = process.env.BASE_URL || "http://localhost:5173";

export default defineConfig({
  testDir: "../e2e/web",
  timeout: 30_000,
  retries: process.env.CI ? 1 : 0,
  // Four workers everywhere (local docker-compose flow и staging CI). Tests
  // own their data via generated unique IIN/email/telegram_user_id, so
  // parallel workers stay isolated on the same DB. Bumped from 2 → 4 alongside
  // admin-creators-list spec to keep staging CI duration manageable as the
  // suite grows.
  workers: 4,
  reporter: [["html", { open: "never" }]],
  use: {
    baseURL: BASE_URL,
    headless: true,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    extraHTTPHeaders:
      process.env.CF_ACCESS_CLIENT_ID
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
  webServer: process.env.CI
    ? undefined
    : [
        {
          command:
            "cd ../../backend && ENVIRONMENT=local DATABASE_URL='postgres://ugcboost:ugcboost_dev@localhost:5433/ugcboost?sslmode=disable' JWT_SECRET='test-secret' ADMIN_PASSWORD='admin123' JWT_EXPIRY='15m' CORS_ORIGINS='http://localhost:5173' PORT=8080 go run ./cmd/api/",
          port: 8080,
          reuseExistingServer: true,
          timeout: 30_000,
        },
        {
          command: "npx vite dev --port 5173",
          port: 5173,
          reuseExistingServer: true,
          timeout: 15_000,
        },
      ],
});
