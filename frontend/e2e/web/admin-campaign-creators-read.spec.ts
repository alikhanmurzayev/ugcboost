/**
 * Browser e2e — admin-секция «Креаторы кампании» на `/campaigns/:campaignId`,
 * read-only-аспекты chunk 11.
 *
 * Закрывает наблюдаемое поведение секции, которое осталось read-only после
 * slice 2/2: отрисовка таблицы с двумя добавленными креаторами через A3 +
 * `listCreators({ids})`, empty-state, переход в `CreatorDrawer` по клику
 * по строке. Mutations Add/Remove живут в `admin-campaign-creators-mutations.spec.ts`
 * (slice 2/2). После slice 2/2 кнопка «Добавить креаторов» — enabled, и
 * её активация проверяется здесь только как факт «не disabled, без
 * tooltip-заглушки»; полный flow открытия drawer'а покрыт mutations spec'ом.
 *
 * Тесты сеют admin'а, кампанию и нужное число одобренных креаторов через
 * composable-хелперы api.ts; рядом с UI-flow остаются прямые вызовы
 * `addCampaignCreators` под admin-токеном, чтобы быстро довести кампанию
 * до состояния «N креаторов уже добавлены». Cleanup идёт LIFO через
 * cleanupCampaign / SeededApprovedCreator.cleanup / SeededUser.cleanup и
 * уважает `E2E_CLEANUP=false` для дебаг-прогонов.
 *
 * Happy live + 2 креатора — admin переходит на /campaigns/:id живой
 * кампании, в которую через API уже добавили двух одобренных креаторов.
 * Ожидается секция `campaign-creators-section` с заголовком, счётчиком
 * «2 в кампании» и таблицей `campaign-creators-table` из двух строк
 * (`row-<creatorId>`), в которых видны фактические ФИО (`<lastName> <firstName>`)
 * и город «Алматы». Кнопка `campaign-creators-add-button` enabled —
 * закрывает AC «после slice 2/2 управление активно».
 *
 * Live + 0 креаторов — admin переходит на /campaigns/:id живой кампании
 * без добавленных. Ожидается empty-state `campaign-creators-empty-all`
 * с текстом «Креаторов в кампании пока нет», счётчик отсутствует, кнопка
 * `campaign-creators-add-button` enabled.
 *
 * Click row → drawer — admin кликает строку креатора в секции; ожидается,
 * что URL обновляется до `?creatorId=<uuid>` и открывается существующий
 * `CreatorDrawer` с подтянутыми detail-полями (имя в заголовке) — то есть
 * страница успешно вызвала getCreator под admin-токеном. Кнопка
 * `drawer-close` закрывает drawer и убирает creatorId из URL.
 *
 * Soft-deleted campaign — секция `campaign-creators-section` не должна
 * рендериться вовсе. Production-эндпоинта soft-delete'а кампании в main
 * пока нет, поэтому путь закрыт unit-тестом `CampaignDetailPage.test.tsx`.
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
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin campaign creators — read-only section behavior", () => {
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
    const socialAHandle = creatorA.application.socials[0]?.handle;
    expect(socialAHandle, "seedApprovedCreator must seed at least one social").toBeDefined();
    await expect(socialA).toHaveAttribute(
      "href",
      `https://instagram.com/${socialAHandle}`,
    );
    await expect(socialA).toContainText(`@${socialAHandle}`);

    const rowB = page.getByTestId(`row-${creatorB.creatorId}`);
    await expect(rowB).toContainText(
      `${creatorB.application.lastName} ${creatorB.application.firstName}`,
    );
    await expect(rowB).toContainText("Алматы");
    await expect(rowB).toContainText("Бьюти (макияж, уход)");

    const addBtn = page.getByTestId("campaign-creators-add-button");
    await expect(addBtn).toBeEnabled();
  });

  test("Live campaign + 0 creators — empty state, counter absent, Add button enabled", async ({
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
      page.getByTestId("campaign-creators-empty-all"),
    ).toHaveText("Креаторов в кампании пока нет");
    await expect(page.getByTestId("campaign-creators-counter")).toHaveCount(0);

    const addBtn = page.getByTestId("campaign-creators-add-button");
    await expect(addBtn).toBeEnabled();
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
    // never fires. After chunk 13 the first cell is the checkbox column
    // (also stopPropagation), so target the index cell at nth(1).
    await row.locator("td").nth(1).click();

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
