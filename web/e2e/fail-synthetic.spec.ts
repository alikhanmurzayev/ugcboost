import { test, expect } from "@playwright/test";

// Synthetic test — must fail. Delete after verifying CI reports.
test("SYNTHETIC FAIL — delete me after CI check", async ({ page }) => {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "This Does Not Exist" })).toBeVisible();
});
