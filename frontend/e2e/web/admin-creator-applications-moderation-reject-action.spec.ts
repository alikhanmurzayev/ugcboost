/**
 * Browser e2e — admin отклонение заявки целиком из drawer'а на moderation-экране.
 *
 * Покрывает action-плоскость reject в статусе moderation: shape moderation-
 * экрана и drawer'а закрыт соседним spec'ом
 * (admin-creator-applications-moderation.spec.ts), здесь — три сценария
 * вокруг кнопки «Отклонить заявку» в footer-bar drawer'а, ровно тот же
 * паттерн что reject на verification, но с `fromStatus=moderation` и
 * setup'ом через IG-auto-webhook.
 *
 * Все сценарии используют IG-only заявки: seedCreatorApplication →
 * (опц.) linkTelegramToApplication → fetchApplicationDetail для
 * verificationCode → triggerSendPulseInstagramWebhook. После успешного
 * webhook'а заявка автоматически переходит в moderation благодаря бэкенду
 * (chunk 8/9) — проверяется fetchApplicationDetail после webhook'а перед
 * UI-сценарием.
 *
 * Happy с TG — IG-only заявка с привязанным Telegram. Логинимся в UI,
 * заходим на /moderation, открываем drawer; в footer-bar видна reject-
 * кнопка. Захватываем ISO-таймстемп ДО клика, открываем confirm-modal
 * (TG-вариант body), нажимаем Submit. После 200 drawer закрывается, `?id=`
 * уходит, строка пропадает с moderation-экрана (заявка ушла в rejected).
 * Admin GET возвращает status=rejected, rejection.fromStatus=moderation,
 * rejection.rejectedByUserId=adminID. Параллельно поллим
 * /test/telegram/sent?chatId=...&since=... 5 секунд и требуем хотя бы одно
 * сообщение в этот chat — отправляет chunk-14 нотифайер. Точный текст не
 * сверяем: контракт chunk-14 backend e2e.
 *
 * Happy без TG — IG-only заявка БЕЗ linkTelegramToApplication. Открываем
 * drawer, в confirm-modal body виден warning-вариант «уведомление об
 * отклонении не будет отправлено». Submit; admin GET → status=rejected,
 * rejection.fromStatus=moderation. Telegram-канал поллится по сидованной
 * (но не привязанной) пар-userId; ждём пустой результат — нотифайер
 * подавляет отправку при отсутствии telegram_link.
 *
 * Cancel — IG-only заявка с TG. Открываем drawer → click reject-button →
 * confirm-modal → cancel. Modal закрывается, drawer остаётся, `?id=`
 * сохраняется. Admin GET подтверждает: статус всё ещё moderation,
 * rejection отсутствует.
 *
 * Cleanup-стек draining'уется в afterEach с per-call 5s таймаутом —
 * захламление БД flak'ает соседние воркеры.
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
  uniqueTelegramUserId,
  type LinkedTelegram,
  type SeededCreatorApplication,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";
import { collectTelegramSent } from "../helpers/telegram";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const TG_WINDOW_MS = 5_000;
const NAV_LINK_MODERATION = "nav-link-creator-applications/moderation";

test.describe("Admin reject application action — moderation", () => {
  // timezoneId pinned for parity with the moderation shape spec, even
  // though this file does not assert on rendered dates — keeps the
  // suite-level invariant ("both moderation specs run in UTC") symmetric
  // and shields future date-dependent assertions added here.
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

  test("Happy with TG — reject moves moderation app to rejected, notifier sends a Telegram message", async ({
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
    expect(before.rejection ?? null).toBeNull();

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

    const rejectBtn = drawer.getByTestId("reject-button");
    await expect(rejectBtn).toBeVisible();
    await expect(rejectBtn).toBeEnabled();

    // Capture timestamp BEFORE clicking submit so the /test/telegram/sent
    // poll is scoped strictly to messages sent in response to this reject.
    const since = new Date().toISOString();

    await rejectBtn.click();
    const dialog = page.getByTestId("reject-confirm-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(
      "Креатор получит уведомление в Telegram-боте",
    );

    await dialog.getByTestId("reject-confirm-submit").click();

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
    expect(afterSubmit.status).toBe("rejected");
    expect(afterSubmit.rejection).toBeTruthy();
    expect(afterSubmit.rejection?.fromStatus).toBe("moderation");
    expect(afterSubmit.rejection?.rejectedByUserId).toBe(admin.userId);

    const messages = await collectTelegramSent(
      request,
      API_URL,
      telegramLink.telegramUserId,
      since,
      TG_WINDOW_MS,
    );
    expect(
      messages.length,
      "reject must trigger at least one Telegram message when TG is linked",
    ).toBeGreaterThanOrEqual(1);
  });

  test("Happy without TG — reject succeeds, notifier sends nothing", async ({
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
      { uuid, linkTG: false },
    );
    cleanupStack.push(application.cleanup);

    // Synthetic chatId: no link exists, so any chatId we poll on must
    // remain silent. Unique-per-worker via crypto.randomBytes prevents bleed
    // from a parallel test's notifier.
    const syntheticChatId = uniqueTelegramUserId();

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

    const since = new Date().toISOString();
    await drawer.getByTestId("reject-button").click();
    const dialog = page.getByTestId("reject-confirm-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(
      "уведомление об отклонении не будет отправлено",
    );

    await dialog.getByTestId("reject-confirm-submit").click();

    await expect(page.getByTestId("drawer")).toHaveCount(0);
    await expect(page).not.toHaveURL(/[?&]id=/);

    const afterSubmit = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(afterSubmit.status).toBe("rejected");
    expect(afterSubmit.rejection?.fromStatus).toBe("moderation");

    const messages = await collectTelegramSent(
      request,
      API_URL,
      syntheticChatId,
      since,
      TG_WINDOW_MS,
    );
    expect(
      messages,
      "reject without linked telegram must not produce any send",
    ).toEqual([]);
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

    await drawer.getByTestId("reject-button").click();
    const dialog = page.getByTestId("reject-confirm-dialog");
    await expect(dialog).toBeVisible();

    await dialog.getByTestId("reject-confirm-cancel").click();

    await expect(page.getByTestId("reject-confirm-dialog")).toHaveCount(0);
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
    expect(after.rejection ?? null).toBeNull();
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

// setupModerationViaIG seeds an IG-only creator application, optionally
// links Telegram, then promotes to `moderation` via the SendPulse webhook
// and verifies the post-promotion status. Returns the seeded app + (when
// linked) the telegram identity for poll assertions. linkTG=false is the
// only path to a moderation row without a telegram_link, which the
// "without TG" reject scenario depends on.
//
// The post-webhook status check is load-bearing — webhook returns 200 even
// on no-op (mismatched handle, race), so a silent backend regression would
// otherwise surface as opaque "row not found" with 30s timeout in the
// downstream UI step.
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
