/**
 * Browser e2e — admin-секция «Креаторы кампании», large-scale сценарий:
 * многозаходное добавление через cap=200 и read-side с 200+ креаторов.
 *
 * Закрывает регрессию из ручного раунда ревью PR #86: cap в drawer'е
 * (`useDrawerSelection`, дефолт 200) — это лимит на ОДИН заход submit'а,
 * а не на всю кампанию. Backend `/campaigns/{id}/creators` не имеет
 * верхней границы, поэтому в кампании может быть >200 креаторов после
 * нескольких заходов добавления. До фикса `useCampaignCreators` тянул
 * профили одним вызовом `listCreators({ids})` с `perPage=200`, который
 * backend (`CreatorListIDsMax = 200`) обрезал 422-кой → secitor падал в
 * `ErrorState` сразу после второго submit'а. Фикс — chunked-fetch
 * профилей по 200 ids.
 *
 * Тест seedит admin'а и 210 одобренных креаторов через api.ts (parallel
 * batches), создаёт кампанию, потом проходит сквозной user flow в UI:
 * открыть drawer → отметить все 200 на pages 1–4 (cap reached, hint
 * виден, page 5 disabled) → submit «Добавить (200)» → reload (доказывает
 * что 200 строк подняты chunked-запросом без ErrorState) → открыть drawer
 * снова (200 первых членов идут с бейджом «Добавлен») → отметить
 * оставшиеся 10 на page 5 → submit «Добавить (10)» → counter «210 в
 * кампании», reload остаётся валидным.
 *
 * Cleanup: 210 creators-cleanup + 1 campaign-cleanup идут LIFO через тот
 * же стек что и существующие mutation-spec'и. `removeCampaignCreator`
 * пред-регистрируется на каждый creatorId до того как UI добавит хоть
 * один — helper идемпотентен (404 = no-op), поэтому id, не доехавшие до
 * persisted-state, обходятся без шума. Уважается `E2E_CLEANUP=false` для
 * дебага упавших прогонов; при ручном дебаге не забыть очистить
 * audit_logs / campaign_creators руками после прогона.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type Page, type APIRequestContext } from "@playwright/test";
import {
  loginAsAdmin,
  removeCampaignCreator,
  seedAdmin,
  seedApprovedCreator,
  seedCampaign,
  type SeededApprovedCreator,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 180_000;
const TOTAL_CREATORS = 210;
const SEED_BATCH = 15;
const CLEANUP_BATCH = 20;

test.describe("Admin campaign creators — large-scale (cap-cycle, 200+ members)", () => {
  test.use({ timezoneId: "UTC" });
  // Seeding 210 approved creators serially through 4 HTTP hops each is slow;
  // even parallelized in batches the run dominates per-test time. The plain
  // Playwright default (30s) blows up in seed phase alone.
  test.setTimeout(600_000);

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

  test("admin adds 200 in first batch, reload survives (chunked profiles), then adds remaining via second batch — campaign holds 210 members", async ({
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

    // Per-run prefix bakes test identity into every seeded creator's
    // lastName so we can scope the drawer to our 210 rows via search filter.
    // Without this, parallel workers seeding fresh creators in /creator-
    // applications-moderation-* tests would push our seeds past page 1, and
    // the «mark all on page X» loop would dedup against an already-checked
    // row that had drifted in from another test (counter ends at 99/100).
    const runId = `e2e-${randomUUID().slice(0, 8)}`;

    const creators = await seedApprovedCreatorsParallel(
      request,
      adminToken,
      TOTAL_CREATORS,
      SEED_BATCH,
      runId,
    );
    // Single batched cleanup hook for all 210 creators. LIFO ordering:
    // detach-batch → campaign → creators-batch → admin. Sequential per-id
    // cleanup (LIFO pop one by one) hit the 60s timeout before getting
    // through 210 × 2 HTTP hops, leaving a creators-table mess that broke
    // backend e2e sort tests on the next run.
    cleanupStack.push(() => batchedCleanup(creators.map((c) => c.cleanup)));

    const campaign = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(campaign.cleanup);

    // Same batched approach for campaign_creators FK detach. Idempotent
    // (404 = already gone) so it is safe to register before the UI persists
    // any rows.
    cleanupStack.push(() =>
      batchedCleanup(
        creators.map(
          (c) => () =>
            removeCampaignCreator(
              request,
              API_URL,
              campaign.campaignId,
              c.creatorId,
              adminToken,
            ),
        ),
      ),
    );

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${campaign.campaignId}`);

    await expect(page.getByTestId("campaign-creators-section")).toBeVisible();
    await expect(
      page.getByTestId("campaign-creators-table-empty"),
    ).toHaveText("Креаторов пока нет");

    // ── First batch: cap-fill 200 across pages 1–4 ───────────────────
    await page.getByTestId("campaign-creators-add-button").click();
    await expect(page.getByTestId("add-creators-drawer-body")).toBeVisible();
    await expect(page.getByTestId("add-creators-drawer-counter")).toHaveText(
      "Выбрано: 0 / 200",
    );

    // Scope drawer to our seeded creators only. Backend ILIKEs lastName /
    // firstName / iin / phone / telegram_username / social handle, and our
    // runId prefix is unique enough to leave exactly TOTAL_CREATORS rows
    // visible. The drawer's pagination thus walks only our seeds, eliminating
    // the cross-test race for `created_at desc` ordering.
    await page.getByTestId("drawer-filters-search").fill(runId);
    await expect(page.getByTestId("add-creators-drawer-pagination-info")).toContainText(
      String(Math.ceil(TOTAL_CREATORS / 50)),
    );

    // perPage=50 → 5 pages for 210. Click all 50 checkboxes on pages 1..4,
    // then jump to page 5 to assert the cap hint locks remaining rows.
    for (let p = 1; p <= 4; p++) {
      await checkAllOnCurrentPage(page);
      const expected = p * 50;
      // toContainText, not toHaveText: at page 4 the counter div also holds
      // the cap-hint span («Максимум 200 за одну операцию»), so a strict
      // equality match fails on the merged textContent.
      await expect(
        page.getByTestId("add-creators-drawer-counter"),
      ).toContainText(`Выбрано: ${expected} / 200`);
      if (p < 4) {
        await page.getByTestId("add-creators-drawer-pagination-next").click();
      }
    }
    await expect(page.getByTestId("add-creators-drawer-cap-hint")).toBeVisible();
    await expect(page.getByTestId("add-creators-drawer-cap-hint")).toContainText(
      "Максимум 200",
    );

    // Page 5 — every checkbox must be disabled because the cap has been hit.
    await page.getByTestId("add-creators-drawer-pagination-next").click();
    await expect(
      page.getByTestId("add-creators-drawer-pagination-info"),
    ).toContainText("5");
    const page5Checkboxes = page.locator(
      '[data-testid^="drawer-row-checkbox-"]',
    );
    const page5Count = await page5Checkboxes.count();
    expect(page5Count).toBeGreaterThan(0);
    for (let i = 0; i < page5Count; i++) {
      await expect(page5Checkboxes.nth(i)).toBeDisabled();
    }

    const submit = page.getByTestId("add-creators-drawer-submit");
    await expect(submit).toHaveText("Добавить (200)");
    await submit.click();
    await expect(page.getByTestId("add-creators-drawer-body")).toHaveCount(0);

    // Section must show 200 and not bounce into ErrorState — the chunked
    // profiles fetch is what makes this hold.
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "200 в кампании",
    );

    // Reload simulates F5 — proves chunked profiles fetch survives across
    // navigation, not only fresh after the mutation invalidate.
    await page.reload();
    await expect(page.getByTestId("campaign-creators-section")).toBeVisible();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "200 в кампании",
    );
    // ErrorState fallback would render <ErrorState> with this exact text;
    // its absence is the regression assertion.
    await expect(
      page.getByText("Не удалось загрузить креаторов"),
    ).toHaveCount(0);

    // ── Second batch: pick the remaining 10 via a fresh drawer ──────
    await page.getByTestId("campaign-creators-add-button").click();
    await expect(page.getByTestId("add-creators-drawer-body")).toBeVisible();
    await expect(page.getByTestId("add-creators-drawer-counter")).toHaveText(
      "Выбрано: 0 / 200",
    );
    await page.getByTestId("drawer-filters-search").fill(runId);
    await expect(page.getByTestId("add-creators-drawer-pagination-info")).toContainText(
      String(Math.ceil(TOTAL_CREATORS / 50)),
    );

    // Walk pages until we find enabled checkboxes — the first 200 ids carry
    // the «Добавлен» badge; the remaining 10 are scattered after them in
    // sort=created_at desc order. Click whatever is enabled, up to 10.
    let picked = 0;
    let visitedPages = 0;
    while (picked < 10 && visitedPages < 6) {
      const enabled = page.locator(
        '[data-testid^="drawer-row-checkbox-"]:not([disabled])',
      );
      const enabledCount = await enabled.count();
      for (let i = 0; i < enabledCount && picked < 10; i++) {
        await enabled.nth(i).check();
        picked++;
      }
      visitedPages++;
      const nextBtn = page.getByTestId("add-creators-drawer-pagination-next");
      const nextDisabled = await nextBtn.isDisabled();
      if (picked >= 10 || nextDisabled) break;
      await nextBtn.click();
    }
    expect(picked).toBe(10);

    await expect(page.getByTestId("add-creators-drawer-counter")).toHaveText(
      "Выбрано: 10 / 200",
    );
    await expect(page.getByTestId("add-creators-drawer-submit")).toHaveText(
      "Добавить (10)",
    );
    await page.getByTestId("add-creators-drawer-submit").click();
    await expect(page.getByTestId("add-creators-drawer-body")).toHaveCount(0);

    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "210 в кампании",
    );

    // Final reload: 210 (= 200 + 10, two chunks of 200 + 10 in profile
    // fetcher) must hold through F5.
    await page.reload();
    await expect(page.getByTestId("campaign-creators-counter")).toHaveText(
      "210 в кампании",
    );
    await expect(
      page.getByText("Не удалось загрузить креаторов"),
    ).toHaveCount(0);
  });
});

// Iterates through every checkbox on the visible drawer page and ticks it.
// `userEvent`-style helper but at Playwright level.
async function checkAllOnCurrentPage(page: Page): Promise<void> {
  const checkboxes = page.locator('[data-testid^="drawer-row-checkbox-"]');
  const n = await checkboxes.count();
  for (let i = 0; i < n; i++) {
    const cb = checkboxes.nth(i);
    if (await cb.isDisabled()) continue;
    await cb.check();
  }
}

// Seeds N approved creators in `batchSize` parallel chunks so the test does
// not serialize 210 × ~4 HTTP hops back-to-back. Each creator's lastName
// embeds `runId` so the drawer's search filter can scope the visible rows
// to ones we own — making assertions deterministic against parallel
// workers seeding into the shared creators table.
async function seedApprovedCreatorsParallel(
  request: APIRequestContext,
  adminToken: string,
  count: number,
  batchSize: number,
  runId: string,
): Promise<SeededApprovedCreator[]> {
  const result: SeededApprovedCreator[] = [];
  for (let start = 0; start < count; start += batchSize) {
    const end = Math.min(start + batchSize, count);
    const batch = await Promise.all(
      Array.from({ length: end - start }, (_, j) =>
        seedApprovedCreator(request, API_URL, adminToken, {
          lastName: `${runId}-${start + j}-Иванов`,
        }),
      ),
    );
    result.push(...batch);
  }
  return result;
}

// Runs cleanup callbacks in `CLEANUP_BATCH`-sized parallel chunks. Promise
// .allSettled keeps a single 404 / network blip from aborting the whole
// teardown and stranding rows in the creators table.
async function batchedCleanup(
  fns: Array<() => Promise<void>>,
): Promise<void> {
  for (let i = 0; i < fns.length; i += CLEANUP_BATCH) {
    const slice = fns.slice(i, i + CLEANUP_BATCH);
    await Promise.allSettled(slice.map((fn) => fn()));
  }
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
