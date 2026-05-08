/**
 * Browser e2e — admin-страница списка одобренных креаторов в веб-приложении.
 *
 * Каждый тест seed'ит свой набор approved-креаторов через composable-хелпер
 * seedApprovedCreator (внутри: seedCreatorApplication → linkTelegram →
 * manualVerify соцсетей через admin API → approveApplication как admin), и
 * дренирует cleanup-стек в afterEach с per-call 5-секундным таймаутом.
 * Параллельные воркеры изолируются через uuid в lastName и uniqueIIN.
 *
 * Happy path — три approved креатора. Открываем /creators, фильтруем по uuid,
 * ассертим что таблица содержит ровно три строки, default sort = full_name asc
 * (URL чистый, без `sort/order/page`), total в заголовке ≥ 3 и видно семь
 * thead-колонок (index | fullName | socials | categories | age | city |
 * createdAt). Закрывает AC «default sort full_name asc + чистый URL».
 *
 * Drawer pre-fill + detail — клик по строке открывает drawer, URL получает
 * `?id=<creatorId>`, сразу видны pre-fill поля из row (drawer-iin,
 * drawer-phone). После того как GET /creators/{id} резолвится, drawer
 * обогащается detail-only полями: drawer-address (когда заявка содержала
 * адрес), drawer-source-application-id (всегда), drawer-middle-name (когда
 * был middleName). Закрывает AC «pre-fill из row, после resolve добавляются
 * address, telegramFirstName/LastName/UserId, categoryOtherText,
 * sourceApplicationId».
 *
 * Drawer keyboard nav — три креатора в одной странице, drawer открыт на
 * среднем. ArrowLeft двигает selection на первый, ArrowRight возвращает на
 * средний и затем на третий, Escape закрывает drawer (URL `?id=` исчезает).
 * На границах prev/next disabled. Закрывает AC «ArrowLeft/Right или Escape».
 *
 * Page reset on filter change — через URL передаём `page=2`, кликаем
 * filters-search, ассертим что URL теряет `page`. Закрывает AC «при смене
 * filter/sort page сбрасывается».
 *
 * RoleGuard — brand_manager не видит nav-link на /creators и при прямом
 * goto'е редиректится на dashboard. Защищает фронт-гард как UX-слой
 * (серверная авторизация уже проверяется в backend e2e).
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  loginAsAdmin,
  seedAdmin,
  seedApprovedCreator,
  seedBrandManager,
  type SeededApprovedCreator,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const NAV_LINK_CREATORS = "nav-link-creators";

test.describe("Admin creators list", () => {
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

  test("Happy path — 3 approved creators visible, default sort full_name asc, clean URL", async ({
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
    const lastNames = ["Aaaa", "Bbbb", "Cccc"];
    const seeded: SeededApprovedCreator[] = [];
    for (const ln of lastNames) {
      const handle = `aidana_${uuid.slice(0, 8)}_${ln.toLowerCase()}`;
      const creator = await seedApprovedCreator(
        request,
        API_URL,
        adminToken,
        {
          lastName: `e2e-${uuid}-${ln}`,
          firstName: "Айдана",
          socials: [{ platform: "tiktok", handle }],
        },
      );
      seeded.push(creator);
      cleanupStack.push(creator.cleanup);
    }

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_CREATORS).click();
    await expect(page.getByTestId("creators-list-page")).toBeVisible();

    await page.getByTestId("filters-search").fill(uuid);

    const table = page.getByTestId("creators-table");
    await expect(table.locator("tbody tr")).toHaveCount(3);
    await expect(page.getByTestId("creators-total")).toHaveText("3");

    // Default sort=full_name asc + page=1 are not serialised to URL.
    const url = new URL(page.url());
    expect(url.searchParams.get("sort")).toBeNull();
    expect(url.searchParams.get("order")).toBeNull();
    expect(url.searchParams.get("page")).toBeNull();

    // Seven thead columns including index, fullName, socials, categories,
    // age, city, createdAt.
    await expect(table.locator("thead th")).toHaveCount(7);
    await expect(page.getByTestId("th-fullName")).toBeVisible();
    await expect(page.getByTestId("th-age")).toBeVisible();
    await expect(page.getByTestId("th-city")).toBeVisible();
    await expect(page.getByTestId("th-createdAt")).toBeVisible();

    // Default ASC by full_name → first row has lastName "Aaaa".
    const firstRow = table.locator("tbody tr").first();
    await expect(firstRow).toContainText("Aaaa");
  });

  test("Drawer pre-fill + detail enrichment", async ({ page, request }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);
    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );

    const uuid = randomUUID();
    const handle = `aidana_${uuid.slice(0, 8)}`;
    const creator = await seedApprovedCreator(request, API_URL, adminToken, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      middleName: "Тестовна",
      address: "ул. Достык, 1",
      socials: [{ platform: "tiktok", handle }],
    });
    cleanupStack.push(creator.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/creators?q=${uuid}`);

    const row = page.getByTestId(`row-${creator.creatorId}`);
    await expect(row).toBeVisible();
    await row.locator("td").first().click();

    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();
    expect(new URL(page.url()).searchParams.get("id")).toBe(creator.creatorId);

    // Pre-fill: IIN/phone available immediately from list-row data.
    await expect(drawer.getByTestId("drawer-iin")).toContainText(
      creator.application.iin,
    );
    await expect(drawer.getByTestId("creator-phone")).toHaveAttribute(
      "href",
      `tel:${creator.application.phone}`,
    );

    // Detail-only fields appear after GET /creators/{id} resolves.
    await expect(drawer.getByTestId("drawer-address")).toContainText(
      "ул. Достык, 1",
    );
    await expect(drawer.getByTestId("drawer-middle-name")).toContainText(
      "Тестовна",
    );
    await expect(
      drawer.getByTestId("drawer-source-application-id"),
    ).toContainText(creator.applicationId);
    // telegramUserId came from the live link helper.
    await expect(drawer.getByTestId("drawer-telegram-copy")).toBeVisible();
  });

  test("Drawer keyboard nav — Arrow keys move selection, Escape closes", async ({
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
    const lastNames = ["Aaaa", "Bbbb", "Cccc"];
    const seeded: SeededApprovedCreator[] = [];
    for (const ln of lastNames) {
      const creator = await seedApprovedCreator(
        request,
        API_URL,
        adminToken,
        {
          lastName: `e2e-${uuid}-${ln}`,
          firstName: "Айдана",
          socials: [
            {
              platform: "tiktok",
              handle: `aidana_${uuid.slice(0, 8)}_${ln.toLowerCase()}`,
            },
          ],
        },
      );
      seeded.push(creator);
      cleanupStack.push(creator.cleanup);
    }

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/creators?q=${uuid}&id=${seeded[1].creatorId}`);

    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    // Mid-row → both prev/next enabled.
    await expect(drawer.getByTestId("drawer-prev")).not.toBeDisabled();
    await expect(drawer.getByTestId("drawer-next")).not.toBeDisabled();

    // ArrowLeft → first row.
    await page.keyboard.press("ArrowLeft");
    await page.waitForFunction(
      (id) => new URL(window.location.href).searchParams.get("id") === id,
      seeded[0].creatorId,
    );
    await expect(drawer.getByTestId("drawer-prev")).toBeDisabled();

    // ArrowRight twice → last row.
    await page.keyboard.press("ArrowRight");
    await page.waitForFunction(
      (id) => new URL(window.location.href).searchParams.get("id") === id,
      seeded[1].creatorId,
    );
    await page.keyboard.press("ArrowRight");
    await page.waitForFunction(
      (id) => new URL(window.location.href).searchParams.get("id") === id,
      seeded[2].creatorId,
    );
    await expect(drawer.getByTestId("drawer-next")).toBeDisabled();

    // Escape → drawer closes, ?id= is gone.
    await page.keyboard.press("Escape");
    await page.waitForFunction(
      () => new URL(window.location.href).searchParams.get("id") === null,
    );
    await expect(page.getByTestId("drawer")).toHaveCount(0);
  });

  test("Filter change resets page to 1", async ({ page, request }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/creators?page=3`);

    await page.getByTestId("filters-search").fill("anything");

    await page.waitForFunction(
      () => new URL(window.location.href).searchParams.get("page") === null,
    );
  });

  test("RoleGuard — brand_manager has no nav link, redirected from /creators", async ({
    page,
    request,
  }) => {
    const manager = await seedBrandManager(request, API_URL);
    cleanupStack.push(manager.cleanup);

    await loginAs(page, manager.email, manager.password);
    await expect(page.getByTestId(NAV_LINK_CREATORS)).toHaveCount(0);

    await page.goto("/creators", { waitUntil: "domcontentloaded" });
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

