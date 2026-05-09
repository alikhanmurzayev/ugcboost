/**
 * Browser e2e — admin-страница создания кампании в веб-приложении.
 *
 * Покрывает chunk 8a campaign-roadmap: страница `/campaigns/new` заменила
 * `<ComingSoonPage testid="campaign-new-stub" />`, который ставил chunk 9.
 * Каждый тест сеет своего admin (а где нужно — brand_manager) через
 * composable-хелперы api.ts; UI-созданные кампании дополнительно
 * вычищаются через `cleanupCampaign` (POST /test/cleanup-entity type=campaign)
 * по id, извлечённому из URL после редиректа. Параллельные воркеры
 * изолируются uuid'ом в name + tmaUrl.
 *
 * Happy path — admin с дашборда переходит на /campaigns, кликает CTA
 * «Создать кампанию», вводит непустой name (с trim'ом по краям) и tmaUrl
 * с uuid-маркером, нажимает submit. Ожидается редирект на /campaigns/:id и
 * рендер реальной страницы деталей (data-testid `campaign-detail-page` с
 * h1 равным сохранённому имени), а вернувшись на /campaigns?q=<uuid>,
 * пользователь видит свежесозданную строку — так закрывается AC
 * «invalidate(campaignKeys.all()) + navigate /campaigns/{id}» + sanity,
 * что бэк действительно записал кампанию.
 *
 * Validation empty — empty submit при пустых обоих полях рендерит inline
 * errors `campaign-name-error` и `campaign-tma-url-error`; запрос на
 * /campaigns не уходит. Закрывает AC «trim+non-empty валидация на клиенте».
 *
 * Conflict — сначала seed'им кампанию через POST /campaigns, потом через
 * UI пытаемся создать вторую с тем же name. Бэк отвечает 409
 * CAMPAIGN_NAME_TAKEN, фронт показывает form-level alert
 * `create-campaign-error` с локализованным actionable-текстом из
 * common:errors. Закрывает AC «409 → form-level error из getErrorMessage
 * + значения полей сохранены».
 *
 * Back-link — на пустой форме клик «← К списку кампаний» возвращает на
 * /campaigns. Закрывает AC «back-link ведёт на /campaigns».
 *
 * RoleGuard — brand_manager при прямом goto'е на /campaigns/new
 * редиректится на dashboard. Серверная авторизация уже проверяется в
 * backend e2e; этот сценарий закрывает фронт-гард как UX-слой.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  cleanupCampaign,
  loginAsAdmin,
  seedAdmin,
  seedBrandManager,
  seedCampaign,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin campaign create", () => {
  test.use({ timezoneId: "UTC" });

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

  test("Happy path — CTA → form → submit → /campaigns/:id stub; row visible after refresh", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const name = `e2e-create-${uuid}`;
    const tmaUrl = `https://tma.ugcboost.kz/tz/${uuid.replaceAll("-", "")}`;

    await loginAs(page, admin.email, admin.password);
    await page.goto("/campaigns");
    await page.getByTestId("campaigns-create-button").click();

    await expect(page).toHaveURL("/campaigns/new");
    await expect(page.getByTestId("campaign-create-page")).toBeVisible();

    // Trim semantics: surrounding whitespace must be stripped before submit.
    await page.getByTestId("campaign-name-input").fill(`  ${name}  `);
    await page.getByTestId("campaign-tma-url-input").fill(`  ${tmaUrl}  `);
    await page.getByTestId("create-campaign-submit").click();

    await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}$/i);
    await expect(page.getByTestId("campaign-detail-page")).toBeVisible();
    await expect(page.getByTestId("campaign-detail-title")).toHaveText(name);

    const detailUrl = new URL(page.url());
    const campaignId = detailUrl.pathname.split("/").pop()!;
    cleanupStack.push(() => cleanupCampaign(request, API_URL, campaignId));

    // Sanity: list query refetches after invalidate(campaignKeys.all()) — the
    // freshly created row is searchable by uuid marker.
    await page.goto(`/campaigns?q=${uuid}`);
    const table = page.getByTestId("campaigns-table");
    await expect(table.locator("tbody tr")).toHaveCount(1);
    await expect(table.locator("tbody tr").first()).toContainText(name);
  });

  test("Empty submit — both inline errors, no /campaigns POST", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    let postFired = false;
    await page.route(`${API_URL}/campaigns`, (route) => {
      if (route.request().method() === "POST") postFired = true;
      void route.continue();
    });

    await loginAs(page, admin.email, admin.password);
    await page.goto("/campaigns/new");

    await page.getByTestId("create-campaign-submit").click();

    await expect(page.getByTestId("campaign-name-error")).toHaveText(
      "Введите название кампании",
    );
    await expect(page.getByTestId("campaign-tma-url-error")).toHaveText(
      "Введите ссылку ТЗ",
    );
    await expect(page).toHaveURL("/campaigns/new");
    expect(postFired).toBe(false);
  });

  test("Conflict — duplicate name shows form-level error from common:errors", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);
    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );

    const uuid = randomUUID();
    const seeded = await seedCampaign(request, API_URL, adminToken, {
      name: `e2e-dup-${uuid}`,
      tmaUrl: `https://tma.ugcboost.kz/tz/${uuid.replaceAll("-", "")}`,
    });
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto("/campaigns/new");

    await page.getByTestId("campaign-name-input").fill(seeded.name);
    await page
      .getByTestId("campaign-tma-url-input")
      .fill(`https://tma.ugcboost.kz/tz/other-${uuid.replaceAll("-", "")}`);
    await page.getByTestId("create-campaign-submit").click();

    const err = page.getByTestId("create-campaign-error");
    await expect(err).toBeVisible();
    await expect(err).toContainText("Кампания с таким названием уже есть");
    await expect(page).toHaveURL("/campaigns/new");
    await expect(page.getByTestId("campaign-name-input")).toHaveValue(
      seeded.name,
    );
  });

  test("Back-link returns to /campaigns", async ({ page, request }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto("/campaigns/new");

    await page.getByTestId("campaign-create-back").click();
    await expect(page).toHaveURL("/campaigns");
    await expect(page.getByTestId("campaigns-list-page")).toBeVisible();
  });

  test("RoleGuard — brand_manager on /campaigns/new is redirected to dashboard", async ({
    page,
    request,
  }) => {
    const manager = await seedBrandManager(request, API_URL);
    cleanupStack.push(manager.cleanup);

    await loginAs(page, manager.email, manager.password);
    await page.goto("/campaigns/new", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL("/");
    await expect(page.getByTestId("dashboard-page")).toBeVisible();
  });
});

async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  label: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () => reject(new Error(`${label} timed out after ${ms}ms`)),
      ms,
    );
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}
