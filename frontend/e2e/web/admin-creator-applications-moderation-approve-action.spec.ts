/**
 * Browser e2e — admin одобрение заявки целиком из drawer'а на moderation-экране.
 *
 * Покрывает action-плоскость approve в статусе moderation: shape moderation-
 * экрана и drawer'а закрыт соседним spec'ом
 * (admin-creator-applications-moderation.spec.ts), здесь — три сценария
 * вокруг кнопки «Одобрить заявку» в footer-bar drawer'а: happy, cancel и
 * race с прямым backend-approve. Reject-зеркало уже живёт в
 * admin-creator-applications-moderation-reject-action.spec.ts; approve
 * отличается тем, что (а) на 422 эндпоинт возвращает `CREATOR_APPLICATION_
 * NOT_APPROVABLE` вместо reject-кода, (б) confirm-modal без
 * hasTelegram-conditional (на approve привязка TG проверяется на бэке —
 * 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` пробрасывается через
 * единый baner), (в) после успешного approve креатор материализуется в
 * `creators` и chunk-20 нотифайер шлёт в Telegram.
 *
 * Все сценарии используют IG-only заявки: seedCreatorApplication →
 * (опц.) linkTelegramToApplication → fetchApplicationDetail для
 * verificationCode → triggerSendPulseInstagramWebhook. После webhook'а
 * заявка переходит в moderation (chunk 8/9) — состояние пересверяется
 * fetchApplicationDetail перед UI-сценарием.
 *
 * Happy с TG — IG-only заявка с привязанным Telegram. Логинимся в UI,
 * заходим на /moderation, открываем drawer; в footer-bar — reject рядом
 * с активной approve-кнопкой (emerald). Захватываем ISO-таймстемп ДО
 * клика, открываем confirm-modal, нажимаем Submit. После 200 drawer
 * закрывается, `?id=` уходит, строка пропадает с moderation-экрана
 * (заявка ушла в approved). Admin GET возвращает status=approved.
 * Параллельно поллим /test/telegram/sent?chatId=...&since=... 5 секунд и
 * требуем хотя бы одно сообщение в этот chat — отправляет chunk-20
 * нотифайер. Точный текст не сверяем: контракт chunk-20 backend e2e.
 *
 * Cancel — IG-only заявка с TG. Открываем drawer → click approve-button
 * → confirm-modal → cancel. Modal закрывается, drawer остаётся, `?id=`
 * сохраняется. Admin GET подтверждает: статус всё ещё moderation.
 *
 * Race 422 — IG-only заявка с TG. Открываем drawer и confirm-modal в UI.
 * Затем напрямую дёргаем backend `POST /creators/applications/{id}/approve`
 * с тем же admin-токеном — заявка уходит в approved. Возвращаемся в UI,
 * жмём Submit в уже открытом dialog'е → бэк отвечает 422
 * `CREATOR_APPLICATION_NOT_APPROVABLE`. Ожидаем: dialog закрылся, drawer
 * остался открытым (404 close-drawer-семантика срабатывает только на 404,
 * не на 422), `[drawer-api-error]` показывает локализованный текст из
 * common.json (`Заявку нельзя одобрить — статус изменился. Обновите
 * страницу.`); invalidate отработал — строка с заявкой пропала из листа,
 * так как теперь её status=approved.
 *
 * Cleanup-стек draining'уется в afterEach с per-call 5s таймаутом —
 * захламление БД flak'ает соседние воркеры. Approved-заявки пытаются
 * чиститься тем же cleanup-entity (бэк cascade на creator выполняет сам);
 * если cleanup упадёт, withTimeout не даст одной заявке зависнуть весь
 * suite.
 */
import { randomUUID } from "node:crypto";
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import {
  fetchApplicationDetail,
  linkTelegramToApplication,
  loginAsAdmin,
  seedAdmin,
  seedCreatorApplication,
  triggerSendPulseInstagramWebhook,
  type LinkedTelegram,
  type SeededCreatorApplication,
} from "../helpers/api";
import { collectTelegramSent } from "../helpers/telegram";
import type { components } from "../types/test-schema";

type CleanupEntityRequest = components["schemas"]["CleanupEntityRequest"];

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const TG_WINDOW_MS = 5_000;
const NAV_LINK_MODERATION = "nav-link-creator-applications/moderation";

const NOT_APPROVABLE_TEXT =
  "Заявку нельзя одобрить — статус изменился. Обновите страницу.";

test.describe("Admin approve application action — moderation", () => {
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

  test("Happy with TG — approve moves moderation app to approved, notifier sends a Telegram message", async ({
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
    const { application, telegramLink } = await setupModerationViaIG(
      request,
      API_URL,
      adminToken,
      { uuid, linkTG: true },
    );
    cleanupStack.push(application.cleanup);

    if (!telegramLink) throw new Error("expected linked telegram in this test");

    const before = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(before.status).toBe("moderation");

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);

    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(1);
    await page
      .getByTestId(`row-${application.applicationId}`)
      .locator("td")
      .first()
      .click();

    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    const approveBtn = drawer.getByTestId("approve-button");
    await expect(approveBtn).toBeVisible();
    await expect(approveBtn).toBeEnabled();

    // Capture timestamp BEFORE clicking submit so the /test/telegram/sent
    // poll is scoped strictly to messages sent in response to this approve.
    const since = new Date().toISOString();

    await approveBtn.click();
    const dialog = page.getByTestId("approve-confirm-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(
      "Креатор получит уведомление в Telegram-боте",
    );

    // Hook into the approve POST so the response carries the freshly created
    // creatorId — we need it to push a creator-cleanup BEFORE the application-
    // cleanup, since the test cleanup-entity endpoint does not cascade
    // creators on approved applications (it 500s without it). Promise.all
    // guarantees the response listener is registered before the click event
    // is dispatched — a bare `click(); await promise` pattern can race on
    // very fast machines.
    const [approveResp] = await Promise.all([
      page.waitForResponse(
        (resp) =>
          resp.url().includes(
            `/creators/applications/${application.applicationId}/approve`,
          ) && resp.request().method() === "POST",
      ),
      dialog.getByTestId("approve-confirm-submit").click(),
    ]);
    expect(approveResp.status()).toBe(200);
    const approveBody = (await approveResp.json()) as {
      data: { creatorId: string };
    };
    expect(approveBody.data.creatorId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );
    cleanupStack.push(() =>
      cleanupCreator(request, API_URL, approveBody.data.creatorId),
    );

    await expect(page.getByTestId("drawer")).toHaveCount(0);
    await expect(page).not.toHaveURL(/[?&]id=/);
    await expect(
      page.getByTestId(`row-${application.applicationId}`),
    ).toHaveCount(0);

    const afterSubmit = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(afterSubmit.status).toBe("approved");

    const messages = await collectTelegramSent(
      request,
      API_URL,
      telegramLink.telegramUserId,
      since,
      TG_WINDOW_MS,
    );
    expect(
      messages.length,
      "approve must trigger at least one Telegram message when TG is linked",
    ).toBeGreaterThanOrEqual(1);
  });

  test("Cancel — modal closes, drawer stays, server state unchanged", async ({
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
    const { application } = await setupModerationViaIG(
      request,
      API_URL,
      adminToken,
      { uuid, linkTG: true },
    );
    cleanupStack.push(application.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await page
      .getByTestId(`row-${application.applicationId}`)
      .locator("td")
      .first()
      .click();

    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    await drawer.getByTestId("approve-button").click();
    const dialog = page.getByTestId("approve-confirm-dialog");
    await expect(dialog).toBeVisible();

    await dialog.getByTestId("approve-confirm-cancel").click();

    await expect(page.getByTestId("approve-confirm-dialog")).toHaveCount(0);
    await expect(drawer).toBeVisible();
    await expect(page).toHaveURL(
      new RegExp(`[?&]id=${application.applicationId}`),
    );

    const after = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(after.status).toBe("moderation");
  });

  test("Race 422 — backend approves between dialog open and submit, UI surfaces the localized banner", async ({
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
    const { application } = await setupModerationViaIG(
      request,
      API_URL,
      adminToken,
      { uuid, linkTG: true },
    );
    cleanupStack.push(application.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await page
      .getByTestId(`row-${application.applicationId}`)
      .locator("td")
      .first()
      .click();

    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    await drawer.getByTestId("approve-button").click();
    const dialog = page.getByTestId("approve-confirm-dialog");
    await expect(dialog).toBeVisible();

    // Race: backend approve directly with the same admin token while the
    // confirm-dialog is open. The next UI submit must hit the 422 path.
    const raceResp = await request.post(
      `${API_URL}/creators/applications/${application.applicationId}/approve`,
      { headers: { Authorization: `Bearer ${adminToken}` } },
    );
    expect(raceResp.status()).toBe(200);
    const raceBody = (await raceResp.json()) as {
      data: { creatorId: string };
    };
    expect(raceBody.data.creatorId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );
    // Push creator cleanup BEFORE the upcoming UI submit, so the LIFO drain
    // wipes the creator before the application — see Happy with TG above.
    cleanupStack.push(() =>
      cleanupCreator(request, API_URL, raceBody.data.creatorId),
    );

    await dialog.getByTestId("approve-confirm-submit").click();

    await expect(page.getByTestId("approve-confirm-dialog")).toHaveCount(0);
    await expect(drawer).toBeVisible();
    await expect(page.getByTestId("drawer-api-error")).toHaveText(
      NOT_APPROVABLE_TEXT,
    );

    // After invalidate the row must disappear from the moderation list
    // (its status is now `approved`).
    await expect(
      page.getByTestId(`row-${application.applicationId}`),
    ).toHaveCount(0);

    const afterSubmit = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(afterSubmit.status).toBe("approved");
  });
});

interface SetupOpts {
  uuid: string;
  linkTG: boolean;
}

interface SetupResult {
  application: SeededCreatorApplication;
  telegramLink?: LinkedTelegram;
}

// setupModerationViaIG is intentionally duplicated from the reject-action
// spec rather than hoisted into helpers/api.ts: the two specs are the only
// callers, the body is short, and a premature shared helper would complicate
// the (very natural) addition of variants per scenario. Keep the two copies
// in sync until a third caller appears.
async function setupModerationViaIG(
  request: APIRequestContext,
  apiUrl: string,
  adminToken: string,
  opts: SetupOpts,
): Promise<SetupResult> {
  const handle = `aidana_test_${opts.uuid.slice(0, 8)}`;
  const application = await seedCreatorApplication(request, apiUrl, {
    lastName: `e2e-${opts.uuid}-Иванова`,
    firstName: "Айдана",
    socials: [{ platform: "instagram", handle }],
  });

  let telegramLink: LinkedTelegram | undefined;
  if (opts.linkTG) {
    telegramLink = await linkTelegramToApplication(
      request,
      apiUrl,
      application.applicationId,
    );
  }

  const detail = await fetchApplicationDetail(
    request,
    apiUrl,
    application.applicationId,
    adminToken,
  );
  await triggerSendPulseInstagramWebhook(request, apiUrl, {
    username: handle,
    verificationCode: detail.verificationCode,
  });

  const detailAfter = await fetchApplicationDetail(
    request,
    apiUrl,
    application.applicationId,
    adminToken,
  );
  if (detailAfter.status !== "moderation") {
    throw new Error(
      `setupModerationViaIG: expected status=moderation, got ${detailAfter.status}`,
    );
  }

  const result: SetupResult = { application };
  if (telegramLink) result.telegramLink = telegramLink;
  return result;
}

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

// cleanupCreator drops the creator-aggregate row and its child snapshots
// (creator_socials, creator_categories) via the test cleanup-entity
// endpoint. 204 = deleted, 404 = already gone. Anything else throws so the
// failure surfaces immediately.
async function cleanupCreator(
  request: APIRequestContext,
  apiUrl: string,
  creatorId: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const body: CleanupEntityRequest = { type: "creator", id: creatorId };
  const resp = await request.post(`${apiUrl}/test/cleanup-entity`, {
    data: body,
  });
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(
      `cleanupCreator ${creatorId}: ${resp.status()} ${await resp.text()}`,
    );
  }
}
