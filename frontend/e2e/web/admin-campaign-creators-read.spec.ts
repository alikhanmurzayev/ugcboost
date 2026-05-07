/**
 * Browser e2e — admin-секция «Креаторы кампании» в read-only-режиме на
 * `/campaigns/:campaignId`.
 *
 * Закрывает chunk 11 slice 1/2 campaign-roadmap: на странице деталей
 * кампании появилась новая секция, которая показывает, какие креаторы уже
 * прикреплены к кампании (через A3 + listCreators({ids})), но управлять
 * составом ещё нельзя — кнопка «Добавить креаторов» disabled с tooltip
 * «Появится в следующем PR» (это slice 2/2). UI добавления в slice 2/2;
 * пока его нет, тест стоит на A1 (`POST /campaigns/{id}/creators`),
 * вызванном напрямую под admin-токеном через testutil-хелпер
 * `addCampaignCreators`. Каждый тест сеет своего admin'а, кампанию и
 * нужное число одобренных креаторов через composable-хелперы api.ts;
 * cleanup идёт LIFO через cleanupCampaign / SeededApprovedCreator.cleanup
 * / SeededUser.cleanup и уважает E2E_CLEANUP=false для дебаг-прогонов.
 *
 * Happy live + 2 креатора — admin переходит на /campaigns/:id живой
 * кампании, в которую через API уже добавили двух одобренных креаторов.
 * Ожидается секция `campaign-creators-section` с заголовком, счётчиком
 * «2 в кампании» и таблицей `campaign-creators-table` из двух строк
 * (`row-<creatorId>`), в которых видны фактические ФИО (`<lastName> <firstName>`)
 * и город «Алматы» — это закрывает AC «таблица показывает N строк с
 * подтянутыми profile-данными через listCreators({ids})». Кнопка
 * `campaign-creators-add-button` disabled с title `Появится в следующем
 * PR` — закрывает AC про disabled-Add+tooltip в slice 1/2.
 *
 * Live + 0 креаторов — admin переходит на /campaigns/:id живой кампании
 * без добавленных. Ожидается empty-state `campaign-creators-table-empty`
 * с текстом «Креаторов пока нет», счётчик отсутствует, кнопка
 * `campaign-creators-add-button` disabled с тем же tooltip'ом.
 *
 * Click row → drawer — admin кликает строку креатора в секции; ожидается,
 * что URL обновляется до `?creatorId=<uuid>` и открывается существующий
 * `CreatorDrawer` с подтянутыми detail-полями (имя в заголовке) — то есть
 * страница успешно вызвала getCreator под admin-токеном. Кнопка
 * `drawer-close` закрывает drawer и убирает creatorId из URL — закрывает
 * AC про click row → URL → existing CreatorDrawer.
 *
 * Soft-deleted campaign — секция `campaign-creators-section` не должна
 * рендериться вовсе. Production-эндпоинта soft-delete'а кампании в main
 * пока нет, поэтому путь закрыт unit-тестом `CampaignDetailPage.test.tsx`
 * (проверяет, что при `campaign.isDeleted === true` секция отсутствует
 * в DOM, а A3 не вызывается). В e2e оставлены только happy-state'ы и
 * один user-flow перехода через drawer.
 */
import { test, expect, type Page } from "@playwright/test";
import {
  addCampaignCreators,
  loginAsAdmin,
  removeCampaignCreator,
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin campaign creators — read-only section (slice 1/2)", () => {
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

  test("Live campaign + 2 creators — table renders 2 rows with full names and counter; Add button disabled", async ({
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

    // Push order: creators → campaign → detach. LIFO pops detach first
    // (frees campaign_creators FK), then campaign deletion succeeds, then
    // each creator is dropped without an FK chain holding it back.
    const creatorA = await seedApprovedCreator(request, API_URL, adminToken);
    cleanupStack.push(creatorA.cleanup);
    const creatorB = await seedApprovedCreator(request, API_URL, adminToken);
    cleanupStack.push(creatorB.cleanup);

    const campaign = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(campaign.cleanup);

    await addCampaignCreators(request, API_URL, campaign.campaignId, adminToken, [
      creatorA.creatorId,
      creatorB.creatorId,
    ]);
    cleanupStack.push(() =>
      removeCampaignCreator(
        request,
        API_URL,
        campaign.campaignId,
        creatorA.creatorId,
        adminToken,
      ),
    );
    cleanupStack.push(() =>
      removeCampaignCreator(
        request,
        API_URL,
        campaign.campaignId,
        creatorB.creatorId,
        adminToken,
      ),
    );

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    await expect(page.getByTestId("campaign-creators-section")).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "2 в кампании",
    );
    await expect(page.getByTestId(`row-${creatorA.creatorId}`)).toBeVisible();
    await expect(page.getByTestId(`row-${creatorB.creatorId}`)).toBeVisible();

    const rowA = page.getByTestId(`row-${creatorA.creatorId}`);
    await expect(rowA).toContainText(
      `${creatorA.application.lastName} ${creatorA.application.firstName}`,
    );
    // Default seed plants city=almaty (label: «Алматы») and category=beauty
    // (label: «Бьюти (макияж, уход)») — assert both rows render hydrated
    // dictionary labels alongside the social handle so all 7 columns are
    // proven to wire through, not just the name.
    await expect(rowA).toContainText("Алматы");
    await expect(rowA).toContainText("Бьюти (макияж, уход)");
    const socialA = rowA.getByTestId("social-instagram");
    await expect(socialA).toHaveAttribute(
      "href",
      `https://instagram.com/${creatorA.application.socials[0]?.handle ?? ""}`,
    );
    await expect(socialA).toContainText(
      `@${creatorA.application.socials[0]?.handle ?? ""}`,
    );

    const rowB = page.getByTestId(`row-${creatorB.creatorId}`);
    await expect(rowB).toContainText(
      `${creatorB.application.lastName} ${creatorB.application.firstName}`,
    );
    await expect(rowB).toContainText("Алматы");
    await expect(rowB).toContainText("Бьюти (макияж, уход)");

    const addBtn = page.getByTestId("campaign-creators-add-button");
    await expect(addBtn).toBeDisabled();
    await expect(addBtn).toHaveAttribute("title", "Появится в следующем PR");
  });

  test("Live campaign + 0 creators — empty state, counter absent, Add button disabled", async ({
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

    const campaign = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(campaign.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    await expect(page.getByTestId("campaign-creators-section")).toBeVisible();
    await expect(
      page.getByTestId("campaign-creators-table-empty"),
    ).toHaveText("Креаторов пока нет");
    await expect(page.getByTestId("campaign-creators-counter")).toHaveCount(0);

    const addBtn = page.getByTestId("campaign-creators-add-button");
    await expect(addBtn).toBeDisabled();
    await expect(addBtn).toHaveAttribute("title", "Появится в следующем PR");
  });

  test("Click row opens CreatorDrawer with detail; close removes creatorId from URL", async ({
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

    // Same push order as the 2-creator test: creator → campaign → detach.
    const creator = await seedApprovedCreator(request, API_URL, adminToken);
    cleanupStack.push(creator.cleanup);

    const campaign = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(campaign.cleanup);

    await addCampaignCreators(request, API_URL, campaign.campaignId, adminToken, [
      creator.creatorId,
    ]);
    cleanupStack.push(() =>
      removeCampaignCreator(
        request,
        API_URL,
        campaign.campaignId,
        creator.creatorId,
        adminToken,
      ),
    );

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    const row = page.getByTestId(`row-${creator.creatorId}`);
    await expect(row).toBeVisible();
    // Click the index cell — clicking the row's centre lands on the socials
    // column whose link wrapper stops propagation, so the row's onClick
    // never fires.
    await row.locator("td").first().click();

    await expect(page).toHaveURL(
      new RegExp(`creatorId=${creator.creatorId}`),
    );
    await expect(page.getByTestId("drawer")).toBeVisible();
    await expect(page.getByTestId("drawer-full-name")).toContainText(
      `${creator.application.lastName} ${creator.application.firstName}`,
    );

    await page.getByTestId("drawer-close").click();
    await expect(page.getByTestId("drawer")).toHaveCount(0);
    await expect(page).not.toHaveURL(
      new RegExp(`creatorId=${creator.creatorId}`),
    );
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
