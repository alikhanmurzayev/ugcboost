/**
 * Browser e2e — admin-кнопки рассылки приглашений и ремайндеров на
 * `/campaigns/:campaignId`.
 *
 * Закрывает chunk 13 campaign-roadmap: после рефактора `CampaignCreatorsSection`
 * в group-based вид admin теперь видит четыре секции по статусам (planned →
 * invited → declined → agreed) и может нажимать «Разослать приглашение» для
 * `planned`/`declined` или «Разослать ремайндер» для `invited`. Тесты гоняют
 * happy-path notify (две строки переезжают из `planned` в `invited`),
 * partial-success через `POST /test/telegram/spy/fail-next` (один в
 * `undelivered`-списке с именем) и race-422 (бэк перевёл одного в `invited` за
 * нашей спиной — фронт показывает inline-validation-error и подтягивает
 * актуальный статус через invalidate).
 *
 * Setup на каждый тест: seed админа, трёх одобренных креаторов с уникальным
 * runId-префиксом в lastName (чтобы параллельные воркеры не пересекались по
 * списку креаторов), кампания через A1, добавление троих в кампанию через
 * `addCampaignCreators`. Все три chat-id регистрируются как fake-chat в спай-
 * стороне, чтобы TeeSender пропускал реального Telegram-бота — без этого
 * `undelivered` всегда возвращал бы "chat not found" на staging. Cleanup стека
 * LIFO: сначала отпускаем FK через `removeCampaignCreator` (admin-токеном),
 * затем кампания, затем `seedApprovedCreator.cleanup` для каждого креатора, в
 * конце `admin.cleanup`. `E2E_CLEANUP=false` сохраняет данные для дебага.
 */
import { randomUUID } from "node:crypto";
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import {
  addCampaignCreators,
  loginAsAdmin,
  notifyAsAdmin,
  registerFailNext,
  registerFakeChat,
  removeCampaignCreator,
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
  type SeededApprovedCreator,
  type SeededCampaign,
  type SeededUser,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

interface NotifyTestSetup {
  admin: SeededUser;
  adminToken: string;
  creatorA: SeededApprovedCreator;
  creatorB: SeededApprovedCreator;
  creatorC: SeededApprovedCreator;
  campaign: SeededCampaign;
}

test.describe("Admin campaign notify — chunk 13", () => {
  test.describe.configure({ timeout: 120_000 });
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
      // Don't let cleanup failures flip a passing test red.
      try {
        await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
      } catch (err) {
        console.error("[campaign-notify] cleanup step failed:", err);
      }
    }
  });

  test("Notify в planned: 2 строки доставлены и переехали в invited", async ({
    page,
    request,
  }) => {
    const { admin, creatorA, creatorB, creatorC, campaign } =
      await setupNotify(request, cleanupStack);

    // Make every chat synthetic so notify resolves locally without touching
    // upstream Telegram.
    for (const c of [creatorA, creatorB, creatorC]) {
      await registerFakeChat(request, API_URL, c.telegram.telegramUserId);
    }

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    // Initial state: 3 in planned, action button disabled until selection
    // is non-empty.
    await expect(
      page.getByTestId("campaign-creators-group-planned"),
    ).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "3 в кампании",
    );

    const action = page.getByTestId("campaign-creators-group-action-planned");
    await expect(action).toBeDisabled();

    await page
      .getByTestId(`campaign-creator-checkbox-${creatorA.creatorId}`)
      .check();
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorB.creatorId}`)
      .check();
    await expect(action).toBeEnabled();

    await action.click();

    await expect(
      page.getByTestId("campaign-creators-group-result-planned-success"),
    ).toContainText("Доставлено 2");

    // Invalidate fired in onSettled rerenders the groups: A and B moved to
    // invited, C still in planned.
    const invitedGroup = page.getByTestId("campaign-creators-group-invited");
    await expect(invitedGroup).toBeVisible();
    await expect(
      invitedGroup.getByTestId(`row-${creatorA.creatorId}`),
    ).toBeVisible();
    await expect(
      invitedGroup.getByTestId(`row-${creatorB.creatorId}`),
    ).toBeVisible();
    const plannedGroup = page.getByTestId("campaign-creators-group-planned");
    await expect(
      plannedGroup.getByTestId(`row-${creatorC.creatorId}`),
    ).toBeVisible();

    // Selection cleared, button disabled again after onSettled.
    await expect(
      page.getByTestId(`campaign-creator-checkbox-${creatorC.creatorId}`),
    ).not.toBeChecked();
    await expect(
      page.getByTestId("campaign-creators-group-action-planned"),
    ).toBeDisabled();
  });

  test("Partial-success: один в undelivered с именем и причиной bot_blocked", async ({
    page,
    request,
  }) => {
    const { admin, creatorA, creatorB, creatorC, campaign } =
      await setupNotify(request, cleanupStack);

    for (const c of [creatorA, creatorB, creatorC]) {
      await registerFakeChat(request, API_URL, c.telegram.telegramUserId);
    }
    // Force the next send to creatorA to fail with the canonical
    // bot-blocked message — backend MapTelegramErrorToReason classifies it
    // as `bot_blocked`.
    await registerFailNext(request, API_URL, creatorA.telegram.telegramUserId);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    await expect(
      page.getByTestId(`campaign-creator-checkbox-${creatorA.creatorId}`),
    ).toBeVisible();

    await page
      .getByTestId(`campaign-creator-checkbox-${creatorA.creatorId}`)
      .check();
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorB.creatorId}`)
      .check();
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorC.creatorId}`)
      .check();
    await page.getByTestId("campaign-creators-group-action-planned").click();

    const result = page.getByTestId("campaign-creators-group-result-planned-success");
    await expect(result).toContainText("Доставлено 2");
    await expect(result).toContainText("Не доставлен 1");
    const undelivered = page.getByTestId(
      `campaign-creators-group-undelivered-planned-${creatorA.creatorId}`,
    );
    await expect(undelivered).toContainText(
      `${creatorA.application.lastName} ${creatorA.application.firstName}`,
    );
    await expect(undelivered).toContainText("заблокировал(а) бота");

    // Successful sends moved B/C to invited; A stayed in planned because the
    // upstream send failed.
    const invitedGroup = page.getByTestId("campaign-creators-group-invited");
    await expect(invitedGroup).toBeVisible();
    await expect(
      invitedGroup.getByTestId(`row-${creatorB.creatorId}`),
    ).toBeVisible();
    await expect(
      invitedGroup.getByTestId(`row-${creatorC.creatorId}`),
    ).toBeVisible();
    const plannedGroup = page.getByTestId("campaign-creators-group-planned");
    await expect(
      plannedGroup.getByTestId(`row-${creatorA.creatorId}`),
    ).toBeVisible();
  });

  test("Race-422: бэк перевёл одного в invited — inline validation error", async ({
    page,
    request,
  }) => {
    const { admin, adminToken, creatorA, creatorB, creatorC, campaign } =
      await setupNotify(request, cleanupStack);

    for (const c of [creatorA, creatorB, creatorC]) {
      await registerFakeChat(request, API_URL, c.telegram.telegramUserId);
    }

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    // Wait for initial planned state to render.
    await expect(
      page.getByTestId(`campaign-creator-checkbox-${creatorA.creatorId}`),
    ).toBeVisible();

    // Through the admin API: notify creator A alone — backend flips its
    // status to `invited`. Frontend cache still shows him under `planned`.
    await notifyAsAdmin(
      request,
      API_URL,
      campaign.campaignId,
      [creatorA.creatorId],
      adminToken,
    );

    // Now select all 3 in the (stale) planned group and submit. Backend
    // collects creator A as wrong_status → 422 CAMPAIGN_CREATOR_BATCH_INVALID.
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorA.creatorId}`)
      .check();
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorB.creatorId}`)
      .check();
    await page
      .getByTestId(`campaign-creator-checkbox-${creatorC.creatorId}`)
      .check();
    await page.getByTestId("campaign-creators-group-action-planned").click();

    await expect(
      page.getByTestId("campaign-creators-group-result-planned-validation"),
    ).toContainText(/уже в другом статусе/i);

    // Selection must be cleared after a 422 — otherwise rapid re-submit
    // re-sends stale ids and locks the admin in a retry loop.
    await expect(
      page.getByTestId(`campaign-creator-checkbox-${creatorB.creatorId}`),
    ).not.toBeChecked();
    await expect(
      page.getByTestId(`campaign-creator-checkbox-${creatorC.creatorId}`),
    ).not.toBeChecked();

    // Invalidate refreshed the lists — creator A is now under invited.
    const invitedGroup = page.getByTestId("campaign-creators-group-invited");
    await expect(invitedGroup).toBeVisible();
    await expect(
      invitedGroup.getByTestId(`row-${creatorA.creatorId}`),
    ).toBeVisible();
  });
});

async function setupNotify(
  request: APIRequestContext,
  cleanupStack: Array<() => Promise<void>>,
): Promise<NotifyTestSetup> {
  const admin = await seedAdmin(request, API_URL);
  cleanupStack.push(admin.cleanup);
  const adminToken = await loginAsAdmin(
    request,
    API_URL,
    admin.email,
    admin.password,
  );

  const runId = `e2e-${randomUUID().slice(0, 8)}`;
  const creatorA = await seedApprovedCreator(request, API_URL, adminToken, {
    lastName: `${runId}-A-Иванов`,
  });
  cleanupStack.push(creatorA.cleanup);
  const creatorB = await seedApprovedCreator(request, API_URL, adminToken, {
    lastName: `${runId}-B-Иванов`,
  });
  cleanupStack.push(creatorB.cleanup);
  const creatorC = await seedApprovedCreator(request, API_URL, adminToken, {
    lastName: `${runId}-C-Иванов`,
  });
  cleanupStack.push(creatorC.cleanup);

  const campaign = await seedCampaign(request, API_URL, adminToken);
  cleanupStack.push(campaign.cleanup);

  await addCampaignCreators(
    request,
    API_URL,
    campaign.campaignId,
    adminToken,
    [creatorA.creatorId, creatorB.creatorId, creatorC.creatorId],
  );
  // FK-cleanup ahead of seedCampaign / seedApprovedCreator unwinding.
  for (const id of [creatorA.creatorId, creatorB.creatorId, creatorC.creatorId]) {
    cleanupStack.push(() =>
      removeCampaignCreator(request, API_URL, campaign.campaignId, id, adminToken),
    );
  }

  return { admin, adminToken, creatorA, creatorB, creatorC, campaign };
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
