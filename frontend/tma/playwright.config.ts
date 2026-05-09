import { defineConfig } from "@playwright/test";

const BASE_URL = process.env.BASE_URL || "http://localhost:3002";

export default defineConfig({
  testDir: "../e2e/tma",
  timeout: 30_000,
  retries: process.env.CI ? 1 : 0,
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
});
