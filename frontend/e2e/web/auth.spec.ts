/**
 * Auth flow E2E — веб-приложение.
 *
 * Каждый тест сидит своего администратора через seedAdmin (UUID-суффикс в
 * email — изоляция от параллельных воркеров) и дренирует его через
 * POST /test/cleanup-entity {type:"user", id} в afterEach. Cleanup —
 * fail-fast с per-call 5-секундным таймаутом: поломанный бэкенд должен
 * упасть громко и сразу, а не оставлять "пользователь остался в БД, тесты
 * через час станут flaky".
 *
 * Happy path: правильный email и пароль ведут на дашборд, в сайдбаре виден
 * email пользователя и admin-only nav-link на верификацию заявок (его
 * присутствие закрывает роль через структуру навигации, без копирайт-зависимых
 * локаторов вроде "Админ"). Противоположный кейс — неправильный пароль:
 * остаёмся на /login, появляется блок login-error без утечки того, какой
 * именно компонент ошибся (email, пароль, сеть).
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
import { test, expect, type Page } from "@playwright/test";
import { seedAdmin, type SeededUser } from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Auth flow", () => {
  let cleanupStack: Array<() => Promise<void>>;

  test.beforeEach(() => {
    cleanupStack = [];
  });

  test.afterEach(async () => {
    if (process.env.E2E_CLEANUP === "false") return;
    while (cleanupStack.length > 0) {
      const fn = cleanupStack.pop();
      if (!fn) continue;
      await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
    }
  });

  test("Happy login — email + password → dashboard with sidebar", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin);

    // Sidebar carries the seeded email and the admin role label. testid =
    // stable locator, toContainText = end-to-end check that the i18n bundle
    // resolved the right key (LoginPage / DashboardLayout unit tests cover
    // the static keys; this guards the runtime wiring).
    const sidebar = page.getByTestId("sidebar");
    await expect(sidebar).toBeVisible();
    await expect(sidebar).toContainText(admin.email);
    await expect(sidebar).toContainText("Админ");

    // Admin-only nav presence is the structural proof of role: a
    // brand_manager would not see this link (covered in admin verification
    // spec). Combined with /, brands and audit, this captures the full
    // admin-side navigation surface.
    await expect(page.getByTestId("nav-link-/")).toBeVisible();
    await expect(page.getByTestId("nav-link-brands")).toBeVisible();
    await expect(page.getByTestId("nav-link-audit")).toBeVisible();
    await expect(
      page.getByTestId("nav-link-creator-applications/verification"),
    ).toBeVisible();

    // Dashboard renders with localized header — testid pins the container,
    // toContainText pins the rendered copy ("Дашборд" comes from the
    // dashboard i18n bundle).
    const dashboard = page.getByTestId("dashboard-page");
    await expect(dashboard).toBeVisible();
    await expect(dashboard).toContainText("Дашборд");
  });

  test("Wrong password — error shown, stay on login", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByTestId("email-input").fill(admin.email);
    await page.getByTestId("password-input").fill("wrongpassword");
    await page.getByTestId("login-button").click();

    await expect(page).toHaveURL("/login");
    // Error block + exact wording — testid is the locator (per
    // frontend-testing-e2e.md), and the textual assert pins the auth-specific
    // copy that LoginPage.test.tsx unit tests also exercise. The duplication
    // is intentional: unit covers the i18n key resolution, e2e covers the
    // backend-frontend contract that wrong-creds bubbles to this exact UI.
    await expect(page.getByTestId("login-error")).toContainText(
      "Неверный email или пароль",
    );
  });

  test("Session restore — F5 keeps user logged in", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin);

    // Reload page (simulates F5 — token lost from memory, refresh kicks in)
    await page.reload();

    await expect(page).toHaveURL("/");
    const dashboard = page.getByTestId("dashboard-page");
    await expect(dashboard).toBeVisible();
    await expect(dashboard).toContainText("Дашборд");
    await expect(page.getByTestId("sidebar")).toContainText(admin.email);
  });

  test("Logout — redirects to login, session destroyed", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin);

    await page.getByTestId("logout-button").click();
    await expect(page).toHaveURL("/login");

    // Try accessing dashboard — should redirect back to login because the
    // session was destroyed, not just because the in-memory state was wiped.
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL("/login");
  });

  test("Protected route — unauthenticated user redirected to login", async ({
    page,
  }) => {
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL("/login");
  });
});

async function loginAs(page: Page, user: SeededUser): Promise<void> {
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await page.getByTestId("email-input").fill(user.email);
  await page.getByTestId("password-input").fill(user.password);
  await page.getByTestId("login-button").click();
  await expect(page).toHaveURL("/");
}

async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  label: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label} timed out after ${ms}ms`)), ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}
