/**
 * Browser e2e — admin-секции «Подписывают договор» / «Договор подписан» /
 * «Отказались от договора» на `/campaigns/:campaignId`.
 *
 * Закрывает chunk 18 campaign-roadmap: после расширения
 * `CampaignCreatorStatus` enum'а в OpenAPI и clients-regen, web-приложение
 * должно отрисовать три новые секции в pipeline-порядке после `agreed`.
 * Tests гоняют полный flow одной строки через реальный backend: admin
 * приглашает креатора в кампанию, креатор соглашается через TMA, contract-
 * outbox worker запускается синхронно через test-endpoint и переводит
 * `cc.status` → `signing`, затем мы имитируем TrustMe и шлём webhook
 * `status=3` (подписал) либо `status=9` (отказал) — фронт после reload
 * показывает соответствующую группу без mass-action и без trash.
 *
 * Таймлайн событий и почему именно polling: `RunTrustMeOutboxOnce`
 * синхронно завершает Phase 1..3 для одного ряда, но read-after-write через
 * отдельный HTTP-чтение `GET /campaigns/{id}/creators` имеет миллисекундный
 * лаг. `waitForCcStatus` (polling 200мс, лимит 5с) страхует от флаков и от
 * случая, когда worker не подобрал ряд — тест падает с явным сообщением,
 * указывающим на backend-баг, а не на тестовую нестабильность. Webhook
 * шлётся с raw secret из `TRUSTME_WEBHOOK_TOKEN` env (default —
 * `local-dev-trustme-webhook-token`, как у backend/.env), поэтому local
 * прогон через `make test-e2e-frontend` работает out-of-the-box.
 *
 * Setup на каждый тест: seed админа, одобренный креатор с привязанным
 * Telegram (через `seedApprovedCreator`), кампания через A1 + загрузка
 * шаблона договора (требование notify-guard chunk 9a), добавление креатора
 * в кампанию через `addCampaignCreators`, регистрация fake-chat для
 * креатора (без неё notify падает с "chat not found"). Cleanup стека LIFO:
 * сначала отпускаем FK через `removeCampaignCreator`, затем кампания,
 * затем `seedApprovedCreator.cleanup`, в конце `admin.cleanup`. Audit-rows
 * и contracts-rows подчищаются каскадом от cleanup-entity. `E2E_CLEANUP=
 * false` сохраняет данные для дебага.
 */
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import {
  addCampaignCreators,
  CAMPAIGN_CREATOR_STATUS,
  findTrustMeSpyByIIN,
  loginAsAdmin,
  notifyAsAdmin,
  registerFakeChat,
  removeCampaignCreator,
  runTrustMeOutboxOnce,
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
  signInitDataForCreator,
  tmaAgreeCampaign,
  triggerTrustMeWebhook,
  TRUSTME_WEBHOOK_DECLINED,
  TRUSTME_WEBHOOK_SIGNED,
  uploadDummyContractTemplate,
  waitForCcStatus,
  type SeededApprovedCreator,
  type SeededCampaign,
  type SeededUser,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

interface TrustMeTestSetup {
  admin: SeededUser;
  adminToken: string;
  creator: SeededApprovedCreator;
  campaign: SeededCampaign;
}

test.describe("Admin campaign creators TrustMe states — chunk 18", () => {
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
      try {
        await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
      } catch (err) {
        console.error("[admin-campaign-creators-trustme] cleanup failed:", err);
      }
    }
  });

  test("signing → signed: webhook со status=3 переводит в «Договор подписан»", async ({
    page,
    request,
  }) => {
    const { admin, adminToken, creator, campaign } = await setupTrustMeFlow(
      request,
      cleanupStack,
    );

    await driveToSigning(page, request, adminToken, creator, campaign);

    const spyRecord = await findTrustMeSpyByIIN(
      request,
      API_URL,
      creator.application.iin,
    );
    expect(spyRecord.documentId).toMatch(/^spy-[0-9a-f]{10}$/);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    const signingGroup = page.getByTestId("campaign-creators-group-signing");
    await expect(signingGroup).toBeVisible();
    await expect(
      signingGroup.getByTestId(`row-${creator.creatorId}`),
    ).toBeVisible();
    // Read-only: no mass-action button, no trash icon.
    await expect(
      page.getByTestId("campaign-creators-group-action-signing"),
    ).toHaveCount(0);
    await expect(
      page.getByTestId(`campaign-creator-remove-${creator.creatorId}`),
    ).toHaveCount(0);

    await triggerTrustMeWebhook(
      request,
      API_URL,
      spyRecord.documentId,
      TRUSTME_WEBHOOK_SIGNED,
    );
    await waitForCcStatus(
      request,
      API_URL,
      campaign.campaignId,
      creator.creatorId,
      CAMPAIGN_CREATOR_STATUS.SIGNED,
      adminToken,
    );

    await page.reload();
    const signedGroup = page.getByTestId("campaign-creators-group-signed");
    await expect(signedGroup).toBeVisible();
    await expect(
      signedGroup.getByTestId(`row-${creator.creatorId}`),
    ).toBeVisible();
    await expect(
      page.getByTestId("campaign-creators-group-signing"),
    ).toHaveCount(0);
    await expect(
      page.getByTestId("campaign-creators-group-action-signed"),
    ).toHaveCount(0);
    await expect(
      page.getByTestId(`campaign-creator-remove-${creator.creatorId}`),
    ).toHaveCount(0);
    // signed/signing_declined пара — invited-pair + decided. invited-count=1
    // потому что admin звал ровно один раз; decided-at непустой (TMA agree).
    await expect(
      page.getByTestId(`campaign-creator-invited-count-${creator.creatorId}`),
    ).toHaveText("1");
    await expect(
      page.getByTestId(`campaign-creator-decided-at-${creator.creatorId}`),
    ).toHaveText(/\d{1,2}\s+\S+\s+\d{4}/);
  });

  test("signing → signing_declined: webhook со status=9 переводит в «Отказались от договора»", async ({
    page,
    request,
  }) => {
    const { admin, adminToken, creator, campaign } = await setupTrustMeFlow(
      request,
      cleanupStack,
    );

    await driveToSigning(page, request, adminToken, creator, campaign);

    const spyRecord = await findTrustMeSpyByIIN(
      request,
      API_URL,
      creator.application.iin,
    );
    expect(spyRecord.documentId).toMatch(/^spy-[0-9a-f]{10}$/);

    // Симметрично первому тесту: сначала проверяем UI на промежуточной
    // signing-секции, потом дёргаем webhook. Иначе регресс «фронт не
    // обновляется при переходе signing → signing_declined» проскочит — БД
    // станет signing_declined ещё до того, как страница откроется.
    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    const signingGroup = page.getByTestId("campaign-creators-group-signing");
    await expect(signingGroup).toBeVisible();
    await expect(
      signingGroup.getByTestId(`row-${creator.creatorId}`),
    ).toBeVisible();

    await triggerTrustMeWebhook(
      request,
      API_URL,
      spyRecord.documentId,
      TRUSTME_WEBHOOK_DECLINED,
    );
    await waitForCcStatus(
      request,
      API_URL,
      campaign.campaignId,
      creator.creatorId,
      CAMPAIGN_CREATOR_STATUS.SIGNING_DECLINED,
      adminToken,
    );

    await page.reload();
    const declinedGroup = page.getByTestId(
      "campaign-creators-group-signing_declined",
    );
    await expect(declinedGroup).toBeVisible();
    await expect(
      declinedGroup.getByTestId(`row-${creator.creatorId}`),
    ).toBeVisible();
    await expect(
      page.getByTestId("campaign-creators-group-signing"),
    ).toHaveCount(0);
    await expect(
      page.getByTestId("campaign-creators-group-action-signing_declined"),
    ).toHaveCount(0);
    await expect(
      page.getByTestId(`campaign-creator-remove-${creator.creatorId}`),
    ).toHaveCount(0);
  });
});

async function setupTrustMeFlow(
  request: APIRequestContext,
  cleanupStack: Array<() => Promise<void>>,
): Promise<TrustMeTestSetup> {
  const admin = await seedAdmin(request, API_URL);
  cleanupStack.push(admin.cleanup);
  const adminToken = await loginAsAdmin(
    request,
    API_URL,
    admin.email,
    admin.password,
  );

  const creator = await seedApprovedCreator(request, API_URL, adminToken);
  cleanupStack.push(creator.cleanup);

  const campaign = await seedCampaign(request, API_URL, adminToken);
  cleanupStack.push(campaign.cleanup);
  await uploadDummyContractTemplate(
    request,
    API_URL,
    campaign.campaignId,
    adminToken,
  );

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

  await registerFakeChat(request, API_URL, creator.telegram.telegramUserId);

  return { admin, adminToken, creator, campaign };
}

// driveToSigning runs the deterministic backend pipeline planned →
// invited (notify) → agreed (TMA) → signing (outbox-once). Ends when the
// backend confirms cc.status=signing — caller decides whether to render the
// admin UI before flipping the row further.
async function driveToSigning(
  _page: Page,
  request: APIRequestContext,
  adminToken: string,
  creator: SeededApprovedCreator,
  campaign: SeededCampaign,
): Promise<void> {
  await notifyAsAdmin(
    request,
    API_URL,
    campaign.campaignId,
    [creator.creatorId],
    adminToken,
  );

  const initData = await signInitDataForCreator(
    request,
    API_URL,
    creator.telegram.telegramUserId,
  );
  await tmaAgreeCampaign(request, API_URL, campaign.tmaUrl, initData);

  await runTrustMeOutboxOnce(request, API_URL);
  await waitForCcStatus(
    request,
    API_URL,
    campaign.campaignId,
    creator.creatorId,
    CAMPAIGN_CREATOR_STATUS.SIGNING,
    adminToken,
  );
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
