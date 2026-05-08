/**
 * Browser e2e — admin одобряет заявку с одновременным добавлением креатора
 * в выбранные кампании прямо из confirm-dialog approve-flow на moderation-
 * экране. Покрывает happy-путь нового опционального параметра
 * `campaignIds[]`: SearchableMultiselect внутри dialog'а заполняется
 * списком active-кампаний, админ выбирает 2 кампании, нажимает «Одобрить»,
 * dialog закрывается, drawer закрывается, заявка переходит в `approved`,
 * креатор материализуется и обнаруживается в roster'е каждой выбранной
 * кампании со статусом `planned`.
 *
 * Старый flow (без выбора кампаний) уже покрыт соседним spec'ом
 * admin-creator-applications-moderation-approve-action.spec.ts. Здесь —
 * только новая поверхность: множественный выбор + invalidate каждой
 * затронутой `campaign_creators`-кэш-записи. Validation-422 (cap=20,
 * dedupe, CAMPAIGN_NOT_AVAILABLE_FOR_ADD) живёт в backend e2e
 * (TestApproveWithCampaigns) — UI-диалог только пробрасывает actionable
 * `error.Message` через inline-баннер, и эта сторона уже покрыта unit'ами
 * ApproveApplicationDialog.test.tsx.
 *
 * Setup пайплайн зеркалит approve-action.spec: seedCreatorApplication →
 * linkTelegramToApplication → triggerSendPulseInstagramWebhook → подтверждение
 * status=moderation. Перед UI-сценарием поднимаются 2 свежие кампании
 * через seedCampaign, и их id'ы (UUID) ожидаются в multiselect-options.
 *
 * Cleanup в afterEach дренирует LIFO-стек: сначала creator (без него
 * cleanup-entity по приложению 500'ит), затем application, затем кампании
 * (на их FK creators ON DELETE нет — но к этому моменту campaign_creators
 * уже улетели вместе с creator-cascade). Per-call timeout 5s — захламление
 * БД флак'ает соседних воркеров.
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
  removeCampaignCreator,
  seedAdmin,
  seedCampaign,
  seedCreatorApplication,
  triggerSendPulseInstagramWebhook,
} from "../helpers/api";
import { loginAs } from "../helpers/ui-web";
import type { components as testComponents } from "../types/test-schema";
import type { components } from "../types/schema";

type CleanupEntityRequest = testComponents["schemas"]["CleanupEntityRequest"];
type CreatorApprovalResult = components["schemas"]["CreatorApprovalResult"];
type ListCampaignCreatorsResult = components["schemas"]["ListCampaignCreatorsResult"];
type CampaignCreator = components["schemas"]["CampaignCreator"];

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const NAV_LINK_MODERATION = "nav-link-creator-applications/moderation";

test.describe("Admin approve application action — moderation with campaigns", () => {
  test.use({ timezoneId: "UTC" });

  let cleanupStack: Array<() => Promise<void>>;

  test.beforeEach(() => {
    cleanupStack = [];
  });

  test.afterEach(async () => {
    if (process.env.E2E_CLEANUP === "false") return;
    // LIFO cleanup with continue-on-failure semantics: a failing pop must
    // not strand the remaining stack (unattached campaigns / orphan
    // creator rows leak between worker runs and flake adjacent specs).
    const errors: unknown[] = [];
    while (cleanupStack.length > 0) {
      const fn = cleanupStack.pop();
      if (!fn) continue;
      try {
        await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
      } catch (e) {
        errors.push(e);
      }
    }
    if (errors.length > 0) {
      throw new AggregateError(errors, `${errors.length} cleanup steps failed`);
    }
  });

  test("Approve with two campaigns — creator attached to both, dialog closes, drawer closes", async ({
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
    const handle = `aidana_test_${uuid.slice(0, 8)}`;
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "instagram", handle }],
    });
    cleanupStack.push(application.cleanup);

    await linkTelegramToApplication(
      request,
      API_URL,
      application.applicationId,
    );
    const detail = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    await triggerSendPulseInstagramWebhook(request, apiUrl(), {
      username: handle,
      verificationCode: detail.verificationCode,
    });
    const moderationDetail = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    if (moderationDetail.status !== "moderation") {
      throw new Error(
        `expected status=moderation, got ${moderationDetail.status}`,
      );
    }

    const campA = await seedCampaign(request, API_URL, adminToken, {
      name: `e2e-approve-camps-A-${uuid.slice(0, 8)}`,
    });
    cleanupStack.push(campA.cleanup);
    const campB = await seedCampaign(request, API_URL, adminToken, {
      name: `e2e-approve-camps-B-${uuid.slice(0, 8)}`,
    });
    cleanupStack.push(campB.cleanup);

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

    await drawer.getByTestId("approve-button").click();
    const dialog = page.getByTestId("approve-confirm-dialog");
    await expect(dialog).toBeVisible();

    const multiselect = dialog.getByTestId("approve-campaigns-multiselect");
    await expect(multiselect).toBeVisible();
    await multiselect.click();

    // Wait for both options to be in the dropdown — the listCampaigns call
    // is fired only when the dialog opens, so this also doubles as a barrier
    // that the campaigns query has resolved before we exercise the submit.
    const optionA = dialog.getByTestId(
      `approve-campaigns-multiselect-option-${campA.campaignId}`,
    );
    const optionB = dialog.getByTestId(
      `approve-campaigns-multiselect-option-${campB.campaignId}`,
    );
    await expect(optionA).toBeVisible();
    await expect(optionB).toBeVisible();
    await optionA.click();
    await optionB.click();

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
    const approveBody = (await approveResp.json()) as CreatorApprovalResult;
    const creatorId = approveBody.data.creatorId;
    expect(creatorId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );
    // creators has no ON DELETE CASCADE on the campaign_creators FK to
    // creators(id), so cleanupCreator below would 500 with a constraint
    // violation. Detach both campaign_creators rows first (LIFO drains in
    // reverse push order — these run BEFORE cleanupCreator).
    cleanupStack.push(() => cleanupCreator(request, API_URL, creatorId));
    cleanupStack.push(() =>
      removeCampaignCreator(request, API_URL, campA.campaignId, creatorId, adminToken),
    );
    cleanupStack.push(() =>
      removeCampaignCreator(request, API_URL, campB.campaignId, creatorId, adminToken),
    );

    await expect(page.getByTestId("approve-confirm-dialog")).toHaveCount(0);
    await expect(page.getByTestId("drawer")).toHaveCount(0);
    await expect(page).not.toHaveURL(/[?&]id=/);

    // Application moved to approved.
    const afterSubmit = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(afterSubmit.status).toBe("approved");

    // Both campaigns now have the creator with status=planned.
    await assertCreatorInCampaign(
      request,
      API_URL,
      adminToken,
      campA.campaignId,
      creatorId,
    );
    await assertCreatorInCampaign(
      request,
      API_URL,
      adminToken,
      campB.campaignId,
      creatorId,
    );
  });
});

function apiUrl(): string {
  return API_URL;
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

async function assertCreatorInCampaign(
  request: APIRequestContext,
  apiUrl: string,
  adminToken: string,
  campaignId: string,
  creatorId: string,
): Promise<void> {
  const resp = await request.get(`${apiUrl}/campaigns/${campaignId}/creators`, {
    headers: { Authorization: `Bearer ${adminToken}` },
  });
  expect(resp.status()).toBe(200);
  const body = (await resp.json()) as ListCampaignCreatorsResult;
  const match = body.data.items.find(
    (row: CampaignCreator) => row.creatorId === creatorId,
  );
  expect(
    match,
    `expected creator ${creatorId} to be attached to campaign ${campaignId}`,
  ).toBeDefined();
  if (!match) return;
  expect(match.status).toBe("planned");
}
