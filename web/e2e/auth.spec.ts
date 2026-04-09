import { test, expect } from "@playwright/test";

// API URL for test setup (seed users). Falls back to docker-compose.test.yml backend.
const API_URL = process.env.API_URL || "http://localhost:8082";
const TEST_EMAIL = `test-web-${Date.now()}@e2e.test`;
const TEST_PASSWORD = "testpass123";

// Seed a test admin user via /test/seed-user before all tests.
test.beforeAll(async ({ request }) => {
  const resp = await request.post(`${API_URL}/test/seed-user`, {
    data: { email: TEST_EMAIL, password: TEST_PASSWORD, role: "admin" },
  });
  expect(resp.status()).toBe(201);
});

test.describe("Auth flow", () => {
  test("1. Happy login — email + password → dashboard with sidebar", async ({
    page,
  }) => {
    await page.goto("/login");

    await page.getByRole("textbox", { name: "Email" }).fill(TEST_EMAIL);
    await page.getByRole("textbox", { name: "Пароль" }).fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Войти" }).click();

    // Should redirect to dashboard
    await expect(page).toHaveURL("/");
    await expect(page.getByRole("heading", { name: "Дашборд" })).toBeVisible();

    // Sidebar shows user info
    await expect(page.getByText(TEST_EMAIL, { exact: true })).toBeVisible();
    await expect(page.getByText("Админ")).toBeVisible();

    // Sidebar navigation present (admin role)
    await expect(page.getByRole("link", { name: "Дашборд" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Кампании" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Модерация" })).toBeVisible();
  });

  test("2. Wrong password — error shown, stay on login", async ({ page }) => {
    await page.goto("/login");

    await page.getByRole("textbox", { name: "Email" }).fill(TEST_EMAIL);
    await page.getByRole("textbox", { name: "Пароль" }).fill("wrongpassword");
    await page.getByRole("button", { name: "Войти" }).click();

    // Should stay on login page with error
    await expect(page).toHaveURL("/login");
    await expect(page.getByRole("alert")).toContainText(
      "Неверный email или пароль",
    );
  });

  test("3. Session restore — F5 keeps user logged in", async ({ page }) => {
    // Login first
    await page.goto("/login");
    await page.getByRole("textbox", { name: "Email" }).fill(TEST_EMAIL);
    await page.getByRole("textbox", { name: "Пароль" }).fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Войти" }).click();
    await expect(page).toHaveURL("/");

    // Reload page (simulates F5 — token lost from memory)
    await page.reload();

    // Should still be on dashboard, not redirected to login
    await expect(page).toHaveURL("/");
    await expect(page.getByRole("heading", { name: "Дашборд" })).toBeVisible();
    await expect(page.getByText(TEST_EMAIL, { exact: true })).toBeVisible();
  });

  test("4. Logout — redirects to login, session destroyed", async ({
    page,
  }) => {
    // Login first
    await page.goto("/login");
    await page.getByRole("textbox", { name: "Email" }).fill(TEST_EMAIL);
    await page.getByRole("textbox", { name: "Пароль" }).fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Войти" }).click();
    await expect(page).toHaveURL("/");

    // Click logout
    await page.getByRole("button", { name: "Выйти" }).click();
    await expect(page).toHaveURL("/login");

    // Try accessing dashboard — should redirect to login
    await page.goto("/");
    await expect(page).toHaveURL("/login");
  });

  test("5. Protected route — unauthenticated user redirected to login", async ({
    page,
  }) => {
    // Go directly to protected route without logging in
    await page.goto("/");
    await expect(page).toHaveURL("/login");
  });
});
