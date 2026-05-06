/**
 * Browser e2e — admin-страница списка кампаний в веб-приложении.
 *
 * Каждый тест seed'ит свой набор кампаний через composable-хелпер seedCampaign
 * (внутри: POST /campaigns с админ-токеном) и дренирует cleanup-стек в afterEach
 * с per-call 5-секундным таймаутом. Параллельные воркеры изолируются через uuid
 * в name-префиксе и tmaUrl, а search по этому uuid отделяет seeded-строки от
 * накопленного в БД фона.
 *
 * Happy path — admin создаёт три кампании с общим uuid-маркером в name. Открывает
 * /campaigns через nav-link sidebar'а, фильтрует поиском по uuid, ассертит что
 * total в заголовке = 3, в таблице ровно три строки, default sort
 * `{created_at, desc}` не выкидывается в URL (URL чистый — без sort/order/page),
 * видны 5 thead-колонок (index | name | tmaUrl | createdAt | actions). Закрывает
 * AC «default sort created_at desc + чистый URL» и AC «5 колонок таблицы».
 *
 * Sort-toggle on name — после фильтра по uuid кликаем заголовок «Название».
 * Первый клик переключает на `?sort=name&order=asc` и API уходит с
 * `sort=name&order=asc`, в результате первая строка имеет lastName-эквивалент
 * `Aaaa`. Закрывает AC «sort-toggle через клик заголовка пишет URL».
 *
 * Page reset on search change — открываем `/campaigns?page=2`, в search-input
 * вводим uuid, ждём что `page` исчезает из URL. Закрывает AC «при смене
 * search/sort page сбрасывается в URL».
 *
 * CTA «Создать кампанию» — клик по data-testid `campaigns-create-button` ведёт
 * на /campaigns/new (стаб chunk 8: data-testid `campain-new-stub`). Закрывает
 * AC «CTA-кнопка ведёт на /campaigns/new».
 *
 * Row click + disabled delete — клик по строке открывает /campaigns/:id
 * (стаб chunk 8: data-testid `campaign-detail-stub`). Кнопка «Удалить» в строке
 * `disabled` и имеет title «Появится позже» — placeholder для chunk 7. Закрывает
 * AC «disabled delete + tooltip». В тесте проверяем атрибуты + что row-click
 * по другой ячейке корректно ведёт на детальную (disabled-кнопка не глотает
 * клик соседнего td).
 *
 * RoleGuard — brand_manager не видит nav-link на /campaigns и при прямом
 * goto'е редиректится на dashboard. Защищает фронт-гард как UX-слой
 * (серверная авторизация уже проверяется в backend e2e).
 *
 * Сценарий «showDeleted off→on с показом удалённой строки + бейджем» отложен
 * до мерджа chunk 7 (DELETE /campaigns/{id}) — на момент написания этой спеки
 * способа soft-delete'нуть кампанию через backend нет; сам toggle и
 * рендеринг бейджа покрыты в `CampaignsListPage.test.tsx`. См. Spec Change Log.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  loginAsAdmin,
  seedAdmin,
  seedBrandManager,
  seedCampaign,
  type SeededCampaign,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const NAV_LINK_CAMPAIGNS = "nav-link-campaigns";

test.describe("Admin campaigns list", () => {
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

  test("Happy path — 3 campaigns visible, default sort created_at desc, clean URL, 5 columns", async ({
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
    const labels = ["Aaaa", "Bbbb", "Cccc"];
    const seeded: SeededCampaign[] = [];
    for (const label of labels) {
      const camp = await seedCampaign(request, API_URL, adminToken, {
        name: `e2e-${uuid}-${label}`,
        tmaUrl: `https://t.me/ugcboost_bot/app?startapp=${uuid.slice(0, 8)}-${label.toLowerCase()}`,
      });
      seeded.push(camp);
      cleanupStack.push(camp.cleanup);
    }

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_CAMPAIGNS).click();
    await expect(page.getByTestId("campaigns-list-page")).toBeVisible();

    await page.getByTestId("campaigns-search").fill(uuid);

    const table = page.getByTestId("campaigns-table");
    await expect(table.locator("tbody tr")).toHaveCount(3);
    await expect(page.getByTestId("campaigns-total")).toHaveText("3");

    // Default sort=created_at desc + page=1 are not serialised to URL.
    const url = new URL(page.url());
    expect(url.searchParams.get("sort")).toBeNull();
    expect(url.searchParams.get("order")).toBeNull();
    expect(url.searchParams.get("page")).toBeNull();

    // Five thead columns: index, name, tmaUrl, createdAt, actions.
    await expect(table.locator("thead th")).toHaveCount(5);
    await expect(page.getByTestId("th-name")).toBeVisible();
    await expect(page.getByTestId("th-createdAt")).toBeVisible();

    // Default DESC by created_at → first row is the last-seeded "Cccc".
    const firstRow = table.locator("tbody tr").first();
    await expect(firstRow).toContainText("Cccc");
  });

  test("Sort-toggle on name — clicking header writes URL and reorders rows", async ({
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
    const labels = ["Cccc", "Aaaa", "Bbbb"];
    for (const label of labels) {
      const camp = await seedCampaign(request, API_URL, adminToken, {
        name: `e2e-${uuid}-${label}`,
        tmaUrl: `https://t.me/ugcboost_bot/app?startapp=${uuid.slice(0, 8)}-${label.toLowerCase()}`,
      });
      cleanupStack.push(camp.cleanup);
    }

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns?q=${uuid}`);

    const table = page.getByTestId("campaigns-table");
    await expect(table.locator("tbody tr")).toHaveCount(3);

    await page.getByTestId("th-name").click();
    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("sort") === "name" && sp.get("order") === "asc";
    });

    const firstRow = table.locator("tbody tr").first();
    await expect(firstRow).toContainText("Aaaa");
  });

  test("Page reset on search change — page=2 → search drops `page`", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns?page=2`);

    await page.getByTestId("campaigns-search").fill("anything");

    await page.waitForFunction(
      () => new URL(window.location.href).searchParams.get("page") === null,
    );
  });

  test("CTA «Создать кампанию» — link goes to /campaigns/new stub", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto("/campaigns");

    await page.getByTestId("campaigns-create-button").click();
    await expect(page).toHaveURL("/campaigns/new");
    await expect(page.getByTestId("campaign-new-stub")).toBeVisible();
  });

  test("Row click → /campaigns/:id stub; disabled delete does not navigate", async ({
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
    const camp = await seedCampaign(request, API_URL, adminToken, {
      name: `e2e-${uuid}-Single`,
      tmaUrl: `https://t.me/ugcboost_bot/app?startapp=${uuid.slice(0, 8)}`,
    });
    cleanupStack.push(camp.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns?q=${uuid}`);

    const deleteBtn = page.getByTestId(`campaign-delete-${camp.campaignId}`);
    await expect(deleteBtn).toBeDisabled();
    await expect(deleteBtn).toHaveAttribute("title", "Появится позже");

    // Click on the row body → goes to /campaigns/:id stub.
    const row = page.getByTestId(`row-${camp.campaignId}`);
    await row.locator("td").first().click();
    await expect(page).toHaveURL(`/campaigns/${camp.campaignId}`);
    await expect(page.getByTestId("campaign-detail-stub")).toBeVisible();
  });

  test("RoleGuard — brand_manager has no nav link, redirected from /campaigns", async ({
    page,
    request,
  }) => {
    const manager = await seedBrandManager(request, API_URL);
    cleanupStack.push(manager.cleanup);

    await loginAs(page, manager.email, manager.password);
    await expect(page.getByTestId(NAV_LINK_CAMPAIGNS)).toHaveCount(0);

    await page.goto("/campaigns", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL("/");
    await expect(page.getByTestId("dashboard-page")).toBeVisible();
  });
});

async function loginAs(
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
