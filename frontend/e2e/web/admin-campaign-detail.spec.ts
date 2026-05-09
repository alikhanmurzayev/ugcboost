/**
 * Browser e2e — admin-страница деталей и редактирования кампании в веб-приложении.
 *
 * Покрывает chunk 8b campaign-roadmap: страница `/campaigns/:campaignId` заменила
 * `<ComingSoonPage testid="campaign-detail-stub" />`, который ставил chunk 8a.
 * Каждый тест сеет своего admin (а где нужно — brand_manager) и нужные кампании
 * через composable-хелперы api.ts; cleanup идёт LIFO через cleanupCampaign /
 * SeededUser.cleanup и уважает E2E_CLEANUP=false для дебаг-прогонов.
 *
 * Happy view — admin переходит на /campaigns/:id уже существующей кампании.
 * Ожидается весь read-only-блок: h1 равно name, секция «О кампании» с полями
 * `campaign-detail-name` / `campaign-detail-tma-url` / `campaign-detail-created-at`
 * / `campaign-detail-updated-at`, кнопка `campaign-edit-button` активна (live —
 * без бейджа «Удалена»), а tmaUrl-ссылка открывает оригинальный URL в новой
 * вкладке (target=_blank, rel=noopener noreferrer). Закрывает AC «view рендерит
 * h1 + поля + edit-кнопку active для live».
 *
 * Happy edit + save + reload — admin кликает «Редактировать», ввод
 * предзаполнен текущими значениями. Меняет name (с trim'ом обрамляющих
 * пробелов) и tmaUrl, нажимает «Сохранить»; ожидается возврат в view (форма
 * исчезла, edit-кнопка снова видна) и обновлённые значения в h1 / в полях.
 * После полной перезагрузки страницы persistence сохраняется — это закрывает
 * AC «204 → invalidate detail+all + isEditing=false + view со свежими
 * значениями» и заодно sanity, что бэк действительно записал PATCH.
 *
 * Validation empty — admin открывает edit, чистит оба поля, нажимает submit.
 * Ожидаются два inline-alert'а (`campaign-edit-name-error`,
 * `campaign-edit-tma-url-error`), и PATCH /campaigns/{id} не уходит — это
 * проверяется через page.route, так что регрессия в trim+non-empty валидации
 * на клиенте поймается без зависимости от поведения бэкенда.
 *
 * Conflict — сидим вторую кампанию через POST /campaigns, потом через UI
 * пытаемся переименовать первую в имя второй. Бэк отвечает 409
 * CAMPAIGN_NAME_TAKEN, фронт показывает `campaign-edit-error` с локализованным
 * actionable-текстом из common:errors, поля сохраняются, edit-режим активен —
 * пользователь может поправить и попробовать снова. Закрывает AC «409 →
 * form-level error через getErrorMessage; поля сохранены; пользователь в
 * edit-режиме».
 *
 * Back-link — на view-режиме клик «← К списку кампаний» возвращает на
 * /campaigns. Закрывает AC «back-link ведёт на /campaigns».
 *
 * Not found — admin переходит на /campaigns/<несуществующий-uuid>; рендерится
 * dedicated `campaign-detail-not-found` state с заголовком «Кампания не
 * найдена», поясняющим текстом и back-link'ом — форма редактирования не
 * рендерится. Закрывает AC «GET 404 → dedicated state + back-link, форма
 * не рендерится».
 *
 * RoleGuard — brand_manager при прямом goto'е на /campaigns/:id
 * редиректится на /. Серверная авторизация уже проверяется в backend e2e;
 * этот сценарий закрывает фронт-гард как UX-слой.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  loginAsAdmin,
  seedAdmin,
  seedBrandManager,
  seedCampaign,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin campaign detail & edit", () => {
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

  test("Happy view — h1, fields, edit button enabled (live campaign)", async ({
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

    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    await expect(page.getByTestId("campaign-detail-page")).toBeVisible();
    await expect(page.getByTestId("campaign-detail-title")).toHaveText(
      seeded.name,
    );
    await expect(page.getByTestId("campaign-detail-name")).toHaveText(
      seeded.name,
    );
    const tmaLink = page.getByTestId("campaign-detail-tma-url");
    await expect(tmaLink).toHaveAttribute("href", seeded.tmaUrl);
    await expect(tmaLink).toHaveAttribute("target", "_blank");
    await expect(tmaLink).toHaveAttribute("rel", "noopener noreferrer");
    await expect(page.getByTestId("campaign-detail-created-at")).toBeVisible();
    await expect(page.getByTestId("campaign-detail-updated-at")).toBeVisible();
    await expect(page.getByTestId("campaign-edit-button")).toBeEnabled();
    await expect(
      page.getByTestId("campaign-detail-deleted-badge"),
    ).toHaveCount(0);
    await expect(page.getByTestId("campaign-edit-form")).toHaveCount(0);
  });

  test("Happy edit + save + reload — trim values, view shows fresh data, persistence after reload", async ({
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

    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    const uuid = randomUUID();
    const newName = `e2e-edit-${uuid}`;
    const newTmaUrl = `https://tma.ugcboost.kz/tz/${uuid.replaceAll("-", "")}`;

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    await page.getByTestId("campaign-edit-button").click();
    await expect(page.getByTestId("campaign-edit-form")).toBeVisible();
    await expect(page.getByTestId("campaign-edit-name-input")).toHaveValue(
      seeded.name,
    );
    await expect(page.getByTestId("campaign-edit-tma-url-input")).toHaveValue(
      seeded.tmaUrl,
    );

    // Fill with surrounding whitespace — trim must strip it before PATCH.
    await page.getByTestId("campaign-edit-name-input").fill(`  ${newName}  `);
    await page
      .getByTestId("campaign-edit-tma-url-input")
      .fill(`  ${newTmaUrl}  `);
    await page.getByTestId("campaign-edit-submit").click();

    await expect(page.getByTestId("campaign-edit-form")).toHaveCount(0);
    await expect(page.getByTestId("campaign-detail-title")).toHaveText(newName);
    await expect(page.getByTestId("campaign-detail-name")).toHaveText(newName);
    await expect(page.getByTestId("campaign-detail-tma-url")).toHaveAttribute(
      "href",
      newTmaUrl,
    );

    // Persistence: a fresh GET (full reload) must surface the same values.
    await page.reload();
    await expect(page.getByTestId("campaign-detail-title")).toHaveText(newName);
    await expect(page.getByTestId("campaign-detail-name")).toHaveText(newName);
    await expect(page.getByTestId("campaign-detail-tma-url")).toHaveAttribute(
      "href",
      newTmaUrl,
    );
  });

  test("Empty submit in edit — both inline errors, no PATCH /campaigns/{id}", async ({
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

    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    let patchFired = false;
    await page.route(`${API_URL}/campaigns/${seeded.campaignId}`, async (route) => {
      if (route.request().method() === "PATCH") patchFired = true;
      await route.continue();
    });

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);
    await page.getByTestId("campaign-edit-button").click();

    await page.getByTestId("campaign-edit-name-input").fill("");
    await page.getByTestId("campaign-edit-tma-url-input").fill("");
    await page.getByTestId("campaign-edit-submit").click();

    await expect(page.getByTestId("campaign-edit-name-error")).toHaveText(
      "Введите название кампании",
    );
    await expect(page.getByTestId("campaign-edit-tma-url-error")).toHaveText(
      "Введите ссылку ТЗ",
    );
    expect(patchFired).toBe(false);
  });

  test("Conflict — duplicate name shows form-level error from common:errors; user stays in edit", async ({
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

    const target = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(target.cleanup);
    const blocker = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(blocker.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${target.campaignId}`);

    await page.getByTestId("campaign-edit-button").click();
    await page.getByTestId("campaign-edit-name-input").fill(blocker.name);
    await page.getByTestId("campaign-edit-submit").click();

    const err = page.getByTestId("campaign-edit-error");
    await expect(err).toBeVisible();
    await expect(err).toContainText("Кампания с таким названием уже есть");
    // Field values preserved, edit form still visible
    await expect(page.getByTestId("campaign-edit-name-input")).toHaveValue(
      blocker.name,
    );
    await expect(page.getByTestId("campaign-edit-form")).toBeVisible();
  });

  test("Back-link returns to /campaigns", async ({ page, request }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);
    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );

    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    await page.getByTestId("campaign-detail-back").click();
    await expect(page).toHaveURL("/campaigns");
    await expect(page.getByTestId("campaigns-list-page")).toBeVisible();
  });

  test("404 — non-existent UUID renders dedicated not-found state, no edit form", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${randomUUID()}`);

    await expect(page.getByTestId("campaign-detail-not-found")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Кампания не найдена" }),
    ).toBeVisible();
    await expect(page.getByTestId("campaign-detail-back")).toHaveAttribute(
      "href",
      "/campaigns",
    );
    await expect(page.getByTestId("campaign-edit-form")).toHaveCount(0);
    await expect(page.getByTestId("campaign-detail-title")).toHaveCount(0);
  });

  test("RoleGuard — brand_manager on /campaigns/:id is redirected to dashboard", async ({
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

    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    const manager = await seedBrandManager(request, API_URL);
    cleanupStack.push(manager.cleanup);

    await loginAs(page, manager.email, manager.password);
    await page.goto(`/campaigns/${seeded.campaignId}`, {
      waitUntil: "domcontentloaded",
    });
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

