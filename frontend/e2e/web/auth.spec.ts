/**
 * Auth flow E2E — веб-приложение.
 *
 * Тесты проходят полный цикл аутентификации одного администратора. В
 * beforeAll сеем тестового пользователя через POST /test/seed-user, а в
 * afterAll удаляем его через DELETE /test/users/${TEST_EMAIL} (если
 * E2E_CLEANUP !== "false"). Все кейсы собраны в одном test.describe
 * ("Auth flow") и работают на общем seeded-админе.
 *
 * Happy path: правильный email и пароль ведут на дашборд, в сайдбаре виден
 * email пользователя, роль «Админ» и полный набор навигационных ссылок
 * (главная, бренды, аудит) — у админа доступ ко всему. Противоположный
 * кейс — неправильный пароль: остаёмся на /login и показываем ошибку
 * «Неверный email или пароль» без утечки того, какой именно компонент
 * ошибся (email, пароль, сеть).
 *
 * Session restore проверяет, что перезагрузка страницы (F5) не выкидывает
 * пользователя на /login: access token в памяти теряется, приложение на
 * старте дёргает refresh через cookie и восстанавливает сессию — мы
 * остаёмся на дашборде с тем же email в сайдбаре. Logout делает обратное:
 * редиректит на /login и по-настоящему уничтожает сессию — попытка зайти
 * на защищённый роут снова улетает на /login, а не пропускает по остатку
 * клиентского состояния. Финальный кейс закрывает защиту роутов с другого
 * конца: неаутентифицированный пользователь, идущий прямо на /, тоже
 * редиректится на /login.
 */
import { test, expect } from "@playwright/test";

// API URL for test setup (seed users). Falls back to docker-compose.test.yml backend.
const API_URL = process.env.API_URL || "http://localhost:8080";
const TEST_EMAIL = `test-web-${Date.now()}@e2e.test`;
const TEST_PASSWORD = "testpass123";

// Seed a test admin user via /test/seed-user before all tests.
test.beforeAll(async ({ request }) => {
  const resp = await request.post(`${API_URL}/test/seed-user`, {
    data: { email: TEST_EMAIL, password: TEST_PASSWORD, role: "admin" },
  });
  expect(resp.status()).toBe(201);
});

test.afterAll(async ({ request }) => {
  if (process.env.E2E_CLEANUP !== "false") {
    await request.delete(`${API_URL}/test/users/${TEST_EMAIL}`);
  }
});

test.describe("Auth flow", () => {
  test("1. Happy login — email + password → dashboard with sidebar", async ({
    page,
  }) => {
    await page.goto("/login");

    await page.getByTestId("email-input").fill(TEST_EMAIL);
    await page.getByTestId("password-input").fill(TEST_PASSWORD);
    await page.getByTestId("login-button").click();

    // Should redirect to dashboard
    await expect(page).toHaveURL("/");
    await expect(page.getByRole("heading", { name: "Дашборд" })).toBeVisible();

    // Sidebar shows user info
    await expect(page.getByText(TEST_EMAIL, { exact: true })).toBeVisible();
    await expect(page.getByText("Админ")).toBeVisible();

    // Sidebar navigation present (admin role)
    await expect(page.getByTestId("nav-link-/")).toBeVisible();
    await expect(page.getByTestId("nav-link-brands")).toBeVisible();
    await expect(page.getByTestId("nav-link-audit")).toBeVisible();
  });

  test("2. Wrong password — error shown, stay on login", async ({ page }) => {
    await page.goto("/login");

    await page.getByTestId("email-input").fill(TEST_EMAIL);
    await page.getByTestId("password-input").fill("wrongpassword");
    await page.getByTestId("login-button").click();

    // Should stay on login page with error
    await expect(page).toHaveURL("/login");
    await expect(page.getByTestId("login-error")).toContainText(
      "Неверный email или пароль",
    );
  });

  test("3. Session restore — F5 keeps user logged in", async ({ page }) => {
    // Login first
    await page.goto("/login");
    await page.getByTestId("email-input").fill(TEST_EMAIL);
    await page.getByTestId("password-input").fill(TEST_PASSWORD);
    await page.getByTestId("login-button").click();
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
    await page.getByTestId("email-input").fill(TEST_EMAIL);
    await page.getByTestId("password-input").fill(TEST_PASSWORD);
    await page.getByTestId("login-button").click();
    await expect(page).toHaveURL("/");

    // Click logout
    await page.getByTestId("logout-button").click();
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
