/**
 * Browser e2e — admin-секция «Креаторы кампании» в режиме mutations Add/Remove
 * на `/campaigns/:campaignId`.
 *
 * Закрывает chunk 11 slice 2/2 campaign-roadmap: после read-only-секции из
 * slice 1/2 admin теперь может управлять составом кампании прямо из UI —
 * добавлять креаторов через drawer (`AddCreatorsDrawer`, который повторяет
 * формат `CreatorsListPage` с фильтрами, чек-боксами первой колонкой и
 * sort=created_at desc по умолчанию) и удалять одного через локальный
 * `RemoveCreatorConfirm`. Тест сеет admin'а и трёх одобренных креаторов
 * через api.ts (composable LIFO cleanup) и кампанию через A1 backend
 * helper'а; затем проходит полный happy-flow: открыть Add drawer, отметить
 * двух из трёх, submit → подтвердить что drawer закрылся и в section
 * появились две строки с ФИО подтянутыми из listCreators({ids}). Reload
 * страницы доказывает, что записи попали в БД (а не остались только в
 * React Query cache); затем клик корзины одной строки открывает
 * `RemoveCreatorConfirm`, confirm — и в таблице остаётся одна строка.
 *
 * Cleanup стека: добавленные через UI campaign_creators-связки удаляются
 * `removeCampaignCreator(API)` для каждого creatorId, чтобы освободить FK
 * до `cleanupCampaign`; `seedApprovedCreator.cleanup` снимает creators-row
 * вместе с originating application; `admin.cleanup` дропает admin-аккаунт.
 * Уважается `E2E_CLEANUP=false` для дебаг-прогонов.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  loginAsAdmin,
  removeCampaignCreator,
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin campaign creators — mutations (slice 2/2)", () => {
  // Happy add/remove on localhost finishes in ~10s, but staging adds real
  // network latency to every drawer open / search-filter / mutation /
  // confirm round-trip and the same test brushes against the 30s default.
  // 2 min ceiling absorbs the staging round-trips with a 10× headroom.
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
      await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
    }
  });

  test("Admin adds two creators via drawer, then removes one via trash confirm", async ({
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

    // Per-run prefix: drawer sort is created_at desc, and the global creators
    // table is shared with parallel test workers — without a unique slug
    // baked into lastName, other tests' fresh seeds out-rank ours in the
    // first-page slice and break our checkbox / badge assertions. Filtering
    // the drawer by `runId` later in the test scopes the visible rows to
    // ones we own.
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

    // Pre-detach hook for whatever the UI ends up persisting. removeCampaignCreator
    // is idempotent (404 → no-op) so it is safe to register cleanups for ids
    // that the test may decide not to add through the UI.
    for (const id of [creatorA.creatorId, creatorB.creatorId, creatorC.creatorId]) {
      cleanupStack.push(() =>
        removeCampaignCreator(request, API_URL, campaign.campaignId, id, adminToken),
      );
    }

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    await expect(page.getByTestId("campaign-creators-section")).toBeVisible();
    await expect(
      page.getByTestId("campaign-creators-table-empty"),
    ).toHaveText("Креаторов пока нет");

    const addBtn = page.getByTestId("campaign-creators-add-button");
    await expect(addBtn).toBeEnabled();
    await addBtn.click();

    await expect(page.getByTestId("add-creators-drawer-body")).toBeVisible();
    await expect(page.getByTestId("drawer-title")).toHaveText(
      "Добавить креаторов",
    );
    await expect(page.getByTestId("add-creators-drawer-counter")).toHaveText(
      "Выбрано: 0 / 200",
    );
    await expect(page.getByTestId("add-creators-drawer-cancel")).toHaveText(
      "Отмена",
    );

    // Scope drawer rows to our own seeds; without this, parallel workers'
    // fresher creators bump ours past page 1 and the checkbox lookups race.
    await page.getByTestId("drawer-filters-search").fill(runId);
    await expect(
      page.getByTestId(`drawer-row-checkbox-${creatorA.creatorId}`),
    ).toBeVisible();

    await page
      .getByTestId(`drawer-row-checkbox-${creatorA.creatorId}`)
      .check();
    await page
      .getByTestId(`drawer-row-checkbox-${creatorB.creatorId}`)
      .check();

    await expect(page.getByTestId("add-creators-drawer-counter")).toHaveText(
      "Выбрано: 2 / 200",
    );
    const submit = page.getByTestId("add-creators-drawer-submit");
    await expect(submit).toHaveText("Добавить (2)");

    await submit.click();

    await expect(page.getByTestId("add-creators-drawer-body")).toHaveCount(0);

    const rowA = page.getByTestId(`row-${creatorA.creatorId}`);
    const rowB = page.getByTestId(`row-${creatorB.creatorId}`);
    await expect(rowA).toBeVisible();
    await expect(rowB).toBeVisible();
    await expect(rowA).toContainText(
      `${creatorA.application.lastName} ${creatorA.application.firstName}`,
    );
    await expect(rowB).toContainText(
      `${creatorB.application.lastName} ${creatorB.application.firstName}`,
    );
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "2 в кампании",
    );

    // Reload simulates F5 — proves rows are persisted in the DB and re-
    // rendered from server state, not only living in React Query memory.
    await page.reload();

    await expect(page.getByTestId(`row-${creatorA.creatorId}`)).toBeVisible();
    await expect(page.getByTestId(`row-${creatorB.creatorId}`)).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "2 в кампании",
    );

    // Reopen the drawer; previously-added creators must render disabled with
    // the «Добавлен» badge — read directly out of A3 via existingCreatorIds.
    await page.getByTestId("campaign-creators-add-button").click();
    await expect(page.getByTestId("add-creators-drawer-body")).toBeVisible();
    await page.getByTestId("drawer-filters-search").fill(runId);
    await expect(
      page.getByTestId(`drawer-row-checkbox-${creatorA.creatorId}`),
    ).toBeVisible();
    for (const member of [creatorA, creatorB]) {
      await expect(
        page.getByTestId(`drawer-row-checkbox-${member.creatorId}`),
      ).toBeDisabled();
      await expect(
        page.getByTestId(`drawer-row-added-badge-${member.creatorId}`),
      ).toHaveText("Добавлен");
    }
    await expect(
      page.getByTestId(`drawer-row-checkbox-${creatorC.creatorId}`),
    ).toBeEnabled();

    await page.getByTestId("add-creators-drawer-cancel").click();
    await expect(page.getByTestId("add-creators-drawer-body")).toHaveCount(0);

    await page
      .getByTestId(`campaign-creator-remove-${creatorA.creatorId}`)
      .click();

    const dialog = page.getByTestId("remove-creator-confirm");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Удалить креатора?");
    await expect(dialog).toContainText(
      `${creatorA.application.lastName} ${creatorA.application.firstName} будет удалён(а) из кампании`,
    );
    await expect(dialog).toContainText("Это действие нельзя отменить.");
    await expect(
      page.getByTestId("remove-creator-confirm-submit"),
    ).toHaveText("Удалить");
    await expect(
      page.getByTestId("remove-creator-confirm-cancel"),
    ).toHaveText("Отмена");

    await page.getByTestId("remove-creator-confirm-submit").click();

    await expect(page.getByTestId("remove-creator-confirm")).toHaveCount(0);
    await expect(page.getByTestId(`row-${creatorA.creatorId}`)).toHaveCount(0);
    await expect(page.getByTestId(`row-${creatorB.creatorId}`)).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "1 в кампании",
    );

    // Remove the last remaining creator and assert empty-state surfaces with
    // the correct copy and the counter disappears.
    await page
      .getByTestId(`campaign-creator-remove-${creatorB.creatorId}`)
      .click();
    await page.getByTestId("remove-creator-confirm-submit").click();
    await expect(page.getByTestId(`row-${creatorB.creatorId}`)).toHaveCount(0);
    await expect(
      page.getByTestId("campaign-creators-table-empty"),
    ).toHaveText("Креаторов пока нет");
    await expect(
      page.getByTestId("campaign-creators-counter"),
    ).toHaveCount(0);
  });

  test("Cancel inside RemoveCreatorConfirm does not invoke the mutation", async ({
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

    const runId = `e2e-${randomUUID().slice(0, 8)}`;
    const creator = await seedApprovedCreator(request, API_URL, adminToken, {
      lastName: `${runId}-Иванов`,
    });
    cleanupStack.push(creator.cleanup);

    const campaign = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(campaign.cleanup);
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
    await page.getByTestId("campaign-creators-add-button").click();
    await page.getByTestId("drawer-filters-search").fill(runId);
    await expect(
      page.getByTestId(`drawer-row-checkbox-${creator.creatorId}`),
    ).toBeVisible();
    await page.getByTestId(`drawer-row-checkbox-${creator.creatorId}`).check();
    await page.getByTestId("add-creators-drawer-submit").click();

    await expect(page.getByTestId(`row-${creator.creatorId}`)).toBeVisible();

    await page
      .getByTestId(`campaign-creator-remove-${creator.creatorId}`)
      .click();
    await page.getByTestId("remove-creator-confirm-cancel").click();

    await expect(page.getByTestId("remove-creator-confirm")).toHaveCount(0);
    await expect(page.getByTestId(`row-${creator.creatorId}`)).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "1 в кампании",
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
