import { defineConfig } from "@playwright/test";

const BASE_URL = process.env.BASE_URL || "http://localhost:5173";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: BASE_URL,
    headless: true,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
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
            "cd ../backend && DATABASE_URL='postgres://ugcboost:ugcboost_dev@localhost:5433/ugcboost?sslmode=disable' JWT_SECRET='test-secret' ADMIN_PASSWORD='admin123' JWT_EXPIRY='15m' PORT=8080 go run ./cmd/api/",
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
