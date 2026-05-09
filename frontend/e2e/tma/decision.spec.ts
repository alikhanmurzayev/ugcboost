/**
 * TMA decision flow E2E — креатор-сторона.
 *
 * Каждый тест строит свежий стек данных от лица креатора и админа: сидим
 * admin'а, approved-креатора и кампанию через API-хелперы, потом A1 (add)
 * + A4 (notify) переводят campaign_creator-строку в `invited`. Подписанный
 * initData приходит из POST /test/tma/sign-init-data и инжектится в URL
 * hash через `addInitScript`, чтобы @telegram-apps/sdk-react подхватил его
 * на старте. Дальше тест идёт на /:secretToken, проходит NDA gate
 * (idempotent — на повторе чекбокса уже нет, хелпер тихо no-op'ит) и
 * взаимодействует с CTA через ConfirmDialog.
 *
 * Покрытие соответствует spec-creator-campaign-decision.md: happy-path
 * invited → agree приводит на AcceptedView без `tma-already-decided-banner`,
 * симметричный invited → decline — на DeclinedView. Идемпотентность
 * проверяется повторным agree после reload — тот же экран AcceptedView, но
 * с видимым баннером. State-machine 422 закрывается сценарием declined →
 * agree: backend отвечает CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE, фронт
 * рендерит inline-ошибку. Ассерт цепляется за `data-error-code` атрибут
 * (стандарт `frontend-testing-e2e.md` § Локаторы запрещает text-based
 * assert'ы на i18n-зависимый копирайт).
 *
 * Cleanup идёт LIFO: campaign_creator (DELETE /campaigns/{id}/creators/{cid})
 * → creator → application → campaign. Каждый шаг идемпотентный (404 = OK).
 * При `E2E_CLEANUP=false` стек остаётся для разбора упавшего сценария.
 */
import { test, expect, type Page } from "@playwright/test";

import {
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
  addCampaignCreators,
  notifyAsAdmin,
  registerFakeChat,
  removeCampaignCreator,
  signInitDataForCreator,
  loginAsAdmin,
  uniqueIIN,
  type SeededUser,
  type SeededApprovedCreator,
  type SeededCampaign,
} from "../helpers/api";
import { mockTelegramWebApp } from "../helpers/tma";

const API_URL = process.env.API_URL || "http://localhost:8082";
const CLEANUP_TIMEOUT_MS = 10_000;

interface InvitedFixture {
  admin: SeededUser;
  adminToken: string;
  campaign: SeededCampaign;
  creator: SeededApprovedCreator;
  secretToken: string;
  initData: string;
  cleanup: () => Promise<void>;
}

// uniqueSecretToken returns a 22-char URL-safe token unique per call —
// fits the `^[A-Za-z0-9_-]{16,}$` regex the backend enforces and stays
// well under the 2048-char tma_url cap.
function uniqueSecretToken(): string {
  return Math.random().toString(36).slice(2, 12) + Date.now().toString(36);
}

// extractSecretToken returns the last path segment of a tma_url.
function extractSecretToken(tmaUrl: string): string {
  const url = new URL(tmaUrl);
  const parts = url.pathname.split("/").filter(Boolean);
  const last = parts[parts.length - 1];
  if (!last) throw new Error(`extractSecretToken: empty last segment in ${tmaUrl}`);
  return last;
}

// withTimeout wraps a promise so cleanup steps cannot hang the suite.
async function withTimeout<T>(p: Promise<T>, ms: number, label: string): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  const timeout = new Promise<T>((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label}: timeout after ${ms}ms`)), ms);
  });
  try {
    return await Promise.race([p, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

// setupInvited composes admin + approved creator + campaign + A1 + A4 so
// the campaign_creator row sits in `invited` status — exactly the precondition
// the TMA decision flow needs.
async function setupInvited(request: import("@playwright/test").APIRequestContext): Promise<InvitedFixture> {
  const admin = await seedAdmin(request, API_URL);
  const adminToken = await loginAsAdmin(request, API_URL, admin.email, admin.password);

  const creator = await seedApprovedCreator(request, API_URL, adminToken, {
    iin: uniqueIIN(),
  });
  // notifyAsAdmin sends through the real Telegram path, but in the test
  // environment we route every send through the spy. Register the
  // creator's actual Telegram chatId so the spy short-circuits the send
  // instead of trying to contact Telegram and 5xx'ing the notify call.
  await registerFakeChat(request, API_URL, creator.telegram.telegramUserId);

  const secretToken = uniqueSecretToken() + "padding16";
  const tmaUrl = `https://tma.ugcboost.kz/tz/${secretToken}`;
  const campaign = await seedCampaign(request, API_URL, adminToken, { tmaUrl });

  await addCampaignCreators(request, API_URL, campaign.campaignId, adminToken, [creator.creatorId]);
  await notifyAsAdmin(request, API_URL, campaign.campaignId, [creator.creatorId], adminToken);

  const initData = await signInitDataForCreator(request, API_URL, creator.telegram.telegramUserId);

  return {
    admin,
    adminToken,
    campaign,
    creator,
    secretToken,
    initData,
    cleanup: async () => {
      await removeCampaignCreator(request, API_URL, campaign.campaignId, creator.creatorId, adminToken).catch(() => {});
      await creator.cleanup().catch(() => {});
      await campaign.cleanup().catch(() => {});
      await admin.cleanup().catch(() => {});
    },
  };
}

async function gotoDecisionPage(page: Page, fx: InvitedFixture) {
  await mockTelegramWebApp(page, fx.initData);
  await page.goto(`/${fx.secretToken}`, { waitUntil: "domcontentloaded" });
  await acceptNda(page);
}

async function acceptNda(page: Page) {
  const checkbox = page.getByTestId("nda-checkbox");
  // NDA is a one-shot gate — on a reload of the same flow it may be gone
  // already. Use isVisible with a short timeout so we never race between
  // count() and check(): if the gate is up we accept it, if not we no-op.
  const visible = await checkbox.isVisible({ timeout: 500 }).catch(() => false);
  if (!visible) return;
  await checkbox.check();
  await page.getByTestId("nda-accept-button").click();
}

test.describe("TMA decision flow", () => {
  let cleanupStack: Array<() => Promise<void>>;

  test.beforeEach(() => {
    cleanupStack = [];
  });

  test.afterEach(async () => {
    if (process.env.E2E_CLEANUP === "false") return;
    while (cleanupStack.length > 0) {
      const fn = cleanupStack.pop();
      if (!fn) continue;
      await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup").catch(() => {});
    }
  });

  test("agree happy path → AcceptedView без already-decided", async ({ page, request }) => {
    const fx = await setupInvited(request);
    cleanupStack.push(fx.cleanup);

    await gotoDecisionPage(page, fx);
    await page.getByTestId("campaign-accept-button").click();
    await page.getByTestId("accept-confirm").click();

    await expect(page.getByTestId("tma-accepted-view")).toBeVisible();
    await expect(page.getByTestId("tma-already-decided-banner")).toHaveCount(0);
  });

  test("decline happy path → DeclinedView без already-decided", async ({ page, request }) => {
    const fx = await setupInvited(request);
    cleanupStack.push(fx.cleanup);

    await gotoDecisionPage(page, fx);
    await page.getByTestId("campaign-decline-button").click();
    await page.getByTestId("decline-confirm").click();

    await expect(page.getByTestId("tma-declined-view")).toBeVisible();
    await expect(page.getByTestId("tma-already-decided-banner")).toHaveCount(0);
  });

  test("agree-after-agree → AcceptedView с already-decided баннером", async ({ page, request }) => {
    const fx = await setupInvited(request);
    cleanupStack.push(fx.cleanup);

    // First agree — flips invited → agreed.
    await gotoDecisionPage(page, fx);
    await page.getByTestId("campaign-accept-button").click();
    await page.getByTestId("accept-confirm").click();
    await expect(page.getByTestId("tma-accepted-view")).toBeVisible();

    // Reload and click agree again — must be the idempotent no-op path.
    await page.goto(`/${fx.secretToken}`, { waitUntil: "domcontentloaded" });
    await acceptNda(page);
    await page.getByTestId("campaign-accept-button").click();
    await page.getByTestId("accept-confirm").click();

    await expect(page.getByTestId("tma-already-decided-banner")).toBeVisible();
  });

  test("declined → agree → ошибка need-reinvite", async ({ page, request }) => {
    const fx = await setupInvited(request);
    cleanupStack.push(fx.cleanup);

    // First decline — flips invited → declined.
    await gotoDecisionPage(page, fx);
    await page.getByTestId("campaign-decline-button").click();
    await page.getByTestId("decline-confirm").click();
    await expect(page.getByTestId("tma-declined-view")).toBeVisible();

    // Reload, try to agree — backend returns 422 CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE.
    await page.goto(`/${fx.secretToken}`, { waitUntil: "domcontentloaded" });
    await acceptNda(page);
    await page.getByTestId("campaign-accept-button").click();
    await page.getByTestId("accept-confirm").click();

    const errorBanner = page.getByTestId("tma-decision-error");
    await expect(errorBanner).toBeVisible();
    await expect(errorBanner).toHaveAttribute(
      "data-error-code",
      "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE",
    );
  });
});
