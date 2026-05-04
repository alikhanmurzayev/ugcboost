/**
 * Browser e2e — admin верификация соцсети вручную из drawer'а заявки.
 *
 * Покрывает только action-плоскость для чанка 11 onboarding-roadmap'а:
 * shape verification-экрана и drawer'а закрыт соседним spec'ом
 * (admin-creator-applications-verification.spec.ts), здесь — три сценария
 * вокруг кнопки «Подтвердить вручную».
 *
 * Все сценарии используют TikTok-only заявки. Это сделано намеренно: бэк
 * (chunk 8/9 — SendPulse webhook) после auto-verify единственной IG-соцсети
 * сразу транзитит заявку в moderation, поэтому собрать сцену «IG=auto,
 * TT=unverified, заявка на verification» через реальные пайплайны
 * нельзя. Сосуществование auto-бейджа и manual-кнопки покрыто unit-
 * тестами ApplicationDrawer / SocialAdminRow с fixture'ом.
 *
 * Happy path — TT-only заявка с привязанным Telegram. Логинимся в UI,
 * открываем drawer; assert: кнопка verify TT enabled. Захватываем
 * ISO-таймстемп (`since`) ДО клика, открываем confirm-модалку, нажимаем
 * Submit. После 200 drawer закрывается, `?id=` уходит, строка пропадает
 * (заявка ушла в moderation). Admin GET /creators/applications/{id}
 * возвращает TT verified=true / method=manual / verifiedByUserId=adminID,
 * статус moderation. Параллельно поллим /test/telegram/sent?chatId=...
 * &since=... 5 секунд и проверяем, что bot НЕ отправлял уведомлений в
 * этот chat — ручная верификация по дизайну тиха для креатора.
 *
 * Cancel — TT-only заявка с привязанным TG. Click verify TT → modal →
 * cancel. Modal закрывается, drawer остаётся, `?id=` сохраняется. Admin
 * GET подтверждает: TT всё ещё false, application status verification.
 *
 * TG не привязан — disabled. Сидим заявку без linkTelegramToApplication.
 * В drawer'е verify-button visible но disabled, рядом visible
 * disabled-hint «Сначала креатор должен привязать Telegram». Force-click
 * по disabled кнопке не открывает confirm-модалку (count = 0).
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
} from "../helpers/api";
import { collectTelegramSent } from "../helpers/telegram";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const TG_SILENCE_WINDOW_MS = 5_000;

test.describe("Admin manual verify action", () => {
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

  test("Happy path — verify TikTok, app moves to moderation, no TG notification", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const ttHandle = `aidana_tt_${uuid.slice(0, 8)}`;
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "tiktok", handle: ttHandle }],
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
    const ttRow = findSocial(before.socials, "tiktok");
    expect(ttRow.verified).toBe(false);

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
    await expect(drawer.getByTestId(`verify-social-${ttRow.id}`)).toBeEnabled();

    // Capture timestamp BEFORE clicking submit so the /test/telegram/sent
    // poll can scope strictly to messages sent in response to this action.
    const since = new Date().toISOString();

    await drawer.getByTestId(`verify-social-${ttRow.id}`).click();
    const dialog = page.getByTestId("verify-confirm-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(`@${ttHandle}`);
    await expect(dialog).toContainText("TikTok");

    await dialog.getByTestId("verify-confirm-submit").click();

    // Drawer closes, ?id= clears, row disappears (app moved to moderation
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
    expect(afterSubmit.status).toBe("moderation");
    const ttAfter = findSocial(afterSubmit.socials, "tiktok");
    expect(ttAfter.verified).toBe(true);
    expect(ttAfter.method).toBe("manual");
    expect(ttAfter.verifiedByUserId).toBe(admin.userId);

    // Telegram silence — manual verification must NOT notify the creator.
    // Poll for the configured window and require zero records throughout.
    const messages = await collectTelegramSent(
      request,
      API_URL,
      tg.telegramUserId,
      since,
      TG_SILENCE_WINDOW_MS,
    );
    expect(messages, "manual verify must not notify the creator").toEqual([]);
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
      socials: [
        { platform: "tiktok", handle: `tt_${uuid.slice(0, 8)}` },
      ],
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
    const before = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    const ttRow = findSocial(before.socials, "tiktok");

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

    await drawer.getByTestId(`verify-social-${ttRow.id}`).click();
    const dialog = page.getByTestId("verify-confirm-dialog");
    await expect(dialog).toBeVisible();

    await dialog.getByTestId("verify-confirm-cancel").click();

    await expect(page.getByTestId("verify-confirm-dialog")).toHaveCount(0);
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
    const ttAfter = findSocial(after.socials, "tiktok");
    expect(ttAfter.verified).toBe(false);
  });

  test("TG not linked — verify button disabled, click ignored", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [
        { platform: "tiktok", handle: `tt_${uuid.slice(0, 8)}` },
      ],
    });
    cleanupStack.push(application.cleanup);

    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );
    const detail = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    const ttRow = findSocial(detail.socials, "tiktok");

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

    const button = drawer.getByTestId(`verify-social-${ttRow.id}`);
    await expect(button).toBeDisabled();
    await expect(
      drawer.getByTestId(`verify-social-${ttRow.id}-disabled-hint`),
    ).toBeVisible();

    // force: true bypasses Playwright's actionability check (which would
    // throw on a disabled button). The point is to assert that even if the
    // user manages to fire a click somehow, the modal stays closed.
    await button.click({ force: true });
    await expect(page.getByTestId("verify-confirm-dialog")).toHaveCount(0);
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

interface DetailSocialLite {
  id: string;
  platform: "instagram" | "tiktok" | "threads";
  handle: string;
  verified: boolean;
  method?: "auto" | "manual" | null;
  verifiedByUserId?: string | null;
}

function findSocial(
  socials: DetailSocialLite[],
  platform: DetailSocialLite["platform"],
): DetailSocialLite {
  const row = socials.find((s) => s.platform === platform);
  if (!row) throw new Error(`social not found for platform ${platform}`);
  return row;
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
