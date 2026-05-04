/**
 * Browser e2e — admin отклонение заявки целиком из drawer'а на verification-экране.
 *
 * Покрывает action-плоскость для фронт-чанка reject (после reorder группы 3
 * roadmap'а — текущий chunk 14): shape verification-экрана и drawer'а закрыт
 * соседним spec'ом (admin-creator-applications-verification.spec.ts), здесь —
 * три сценария вокруг кнопки «Отклонить заявку» в footer-bar drawer'а.
 *
 * Все сценарии используют TT-only заявки и админа, посаженного через
 * /test/seed-user. Заявка остаётся в `verification` статусе ровно потому,
 * что бэк (chunk 8/9 webhook) не получает auto-verify сигнала на TT —
 * единственный путь сменить её статус — manual-verify (chunk 11) либо reject
 * (chunk 12). Здесь мы дёргаем reject через UI и ассертим серверное
 * состояние и наличие/отсутствие отправленного Telegram-сообщения.
 *
 * Happy с TG — TT-only заявка с привязанным Telegram. Логинимся в UI,
 * открываем drawer; в footer-bar видна outlined-red кнопка «Отклонить
 * заявку». Захватываем ISO-таймстемп ДО клика, открываем confirm-modal
 * (TG-вариант body), нажимаем Submit. После 200 drawer закрывается, `?id=`
 * уходит, строка пропадает (заявка ушла в rejected). Admin GET возвращает
 * status=rejected, rejection.fromStatus=verification и
 * rejection.rejectedByUserId=adminID. Параллельно поллим
 * /test/telegram/sent?chatId=...&since=... 5 секунд и требуем
 * хотя бы одно сообщение в этот chat — отправляет chunk-14 нотифайер. Точный
 * текст сообщения не сверяем: это контракт chunk-14 e2e на бэкенде.
 *
 * Happy без TG — TT-only заявка БЕЗ linkTelegramToApplication. Открываем
 * drawer, в confirm-modal body виден warning-вариант «уведомление об
 * отклонении не будет отправлено». Submit; admin GET → status=rejected,
 * rejection.fromStatus=verification. Telegram-канал поллится тем же
 * /test/telegram/sent — но т.к. chat не привязан, мы поллим по сидованной
 * (но не привязанной) пар-userId; ждём пустой результат — нотифайер
 * подавляет отправку при отсутствии telegram_link.
 *
 * Cancel — TT-only заявка с TG. Открываем drawer → click reject-button →
 * confirm-modal → cancel. Modal закрывается, drawer остаётся, `?id=`
 * сохраняется. Admin GET подтверждает: статус всё ещё verification,
 * rejection отсутствует.
 *
 * Cleanup-стек draining'уется в afterEach с 5s timeout per call —
 * захламление БД flak'ает соседние воркеры.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page } from "@playwright/test";
import {
  fetchApplicationDetail,
  linkTelegramToApplication,
  loginAsAdmin,
  seedAdmin,
  seedCreatorApplication,
  uniqueTelegramUserId,
} from "../helpers/api";
import { collectTelegramSent } from "../helpers/telegram";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const TG_WINDOW_MS = 5_000;

test.describe("Admin reject application action", () => {
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

  test("Happy with TG — reject moves app to rejected, notifier sends a Telegram message", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "tiktok", handle: `tt_${uuid.slice(0, 8)}` }],
    });
    cleanupStack.push(application.cleanup);

    const tg = await linkTelegramToApplication(
      request,
      API_URL,
      application.applicationId,
    );

    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );
    const before = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(before.status).toBe("verification");
    expect(before.rejection ?? null).toBeNull();

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
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

    // Drawer closes, ?id= clears, row disappears (app moved to rejected
    // and is no longer on the verification screen).
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
    expect(afterSubmit.rejection?.fromStatus).toBe("verification");
    expect(afterSubmit.rejection?.rejectedByUserId).toBe(admin.userId);

    // Notifier sends one or more messages within the window. Exact text is
    // verified by the chunk-14 backend e2e — here we only assert presence.
    const messages = await collectTelegramSent(
      request,
      API_URL,
      tg.telegramUserId,
      since,
      TG_WINDOW_MS,
    );
    expect(
      messages.length,
      "reject must trigger at least one Telegram message when TG is linked",
    ).toBeGreaterThanOrEqual(1);
  });

  test("Happy without TG — reject succeeds, notifier sends nothing (no link)", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "tiktok", handle: `tt_${uuid.slice(0, 8)}` }],
    });
    cleanupStack.push(application.cleanup);

    // Synthetic chatId for polling: the application has no link, so any
    // chatId we poll on must remain silent. This number is unique per worker
    // via crypto.randomBytes, so a real notifier from a parallel test cannot
    // bleed into this poll's seen-set.
    const syntheticChatId = uniqueTelegramUserId();

    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
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
    expect(afterSubmit.rejection?.fromStatus).toBe("verification");

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

    const uuid = randomUUID();
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "tiktok", handle: `tt_${uuid.slice(0, 8)}` }],
    });
    cleanupStack.push(application.cleanup);

    await linkTelegramToApplication(
      request,
      API_URL,
      application.applicationId,
    );

    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
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
    expect(after.status).toBe("verification");
    expect(after.rejection ?? null).toBeNull();
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
