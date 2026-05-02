/**
 * Browser e2e — admin-верификация заявок креаторов в веб-приложении.
 *
 * Каждый тест сидит свой набор данных через API (admin или brand_manager,
 * 0..3 заявок, опционально привязанный TG), прогоняет UI-сценарий и дренирует
 * локальный cleanup-стек в afterEach. uuid в lastName заявок гарантирует, что
 * параллельные воркеры не пересекаются по search-фильтру; при
 * `E2E_CLEANUP=false` всё остаётся в БД для разбора упавшего сценария.
 * Cleanup в afterEach падает на первой же ошибке per-call (Promise.race с
 * 5-секундным таймаутом) — захламление БД ведёт к flaky-collision'ам в
 * следующих прогонах, лучше упасть громко и сразу.
 *
 * Happy path — admin логинится, ищет по uuid, находит ровно одну строку,
 * открывает drawer и проверяет equality на все отображаемые поля:
 * заголовок-ФИО, локализованную строку timeline'а, дату рождения с
 * pluralYears-склонением, ИИН, phone-ссылку с tel:-href'ом, город (canonical
 * "Алматы" из dictionary cities), две category-чипы и одну "Другое: ..." из
 * categoryOtherText, два social-link'а с handle. Telegram-блок показан как
 * "не привязан" с кнопкой копирования сообщения. Click drawer-close убирает
 * drawer и `?id=` из URL.
 *
 * Заявка без отчества — отдельный кейс закрывает контракт helper'а: каллер
 * передаёт middleName=null, helper отправляет в API без middleName, бэк
 * нормализует через trimOptional до nil, drawer строит ФИО через
 * filter(Boolean) и показывает заголовок без trailing-space. Такой же
 * результат для middleName="", который backend схлопывает к nil.
 *
 * Drawer prev/next — три заявки с общим uuid, последовательные POST'ы
 * (helper делает 10ms-задержку перед каждым) задают детерминированный
 * created_at desc даже на медленной CI. Тест начинает с самой свежей, идёт
 * next-кнопкой и ArrowRight'ом до конца (next disabled), возвращается
 * prev-кнопкой и закрывает Escape'ом — закрывает все три способа управления
 * drawer'ом одним сценарием.
 *
 * Filter telegramLinked — две заявки, у одной TG привязан через
 * /test/telegram/message с "/start <id>" (in-process bot handler пишет связь
 * синхронно). Сегмент `any` / `true` / `false` проверен во всех трёх ветках
 * + возврат на `any`, чтобы зафиксировать обратимость фильтра.
 *
 * Empty state — admin вводит свежий random uuid в search; никакая заявка не
 * сидится. `applications-table-empty` виден, сама таблица в DOM отсутствует
 * — закрывает UI-инвариант "пустой результат не рендерит каркас".
 *
 * RoleGuard — brand_manager логинится, в сайдбаре нет nav-link на
 * verification, прямой goto на /creator-applications/verification
 * редиректит на ROUTES.DASHBOARD = "/", где рендерится дашборд.
 */
import { randomUUID } from "node:crypto";
import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import {
  linkTelegramToApplication,
  seedAdmin,
  seedBrandManager,
  seedCreatorApplication,
  type SeededCreatorApplication,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

test.describe("Admin verification flow", () => {
  // Pin browser TZ to UTC so toLocaleDateString / toLocaleString outputs from
  // the rendered drawer match what we compute in Node — both sides format
  // with the same offsets regardless of the host timezone.
  test.use({ timezoneId: "UTC" });

  let cleanupStack: Array<() => Promise<void>>;

  test.beforeEach(() => {
    cleanupStack = [];
  });

  // Cleanup is fail-fast: the first cleanup-fn that errors aborts the drain
  // and re-throws so the next prog catches a hard failure instead of silently
  // leaking rows. Each call is wrapped in a 5s timeout — a hung backend would
  // otherwise block afterEach until Playwright's global 60s ceiling kicks in.
  test.afterEach(async () => {
    if (process.env.E2E_CLEANUP === "false") return;
    while (cleanupStack.length > 0) {
      const fn = cleanupStack.pop();
      if (!fn) continue;
      await withTimeout(fn(), CLEANUP_TIMEOUT_MS, "cleanup");
    }
  });

  test("Happy path — admin opens drawer and sees full applicant data", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    // Resolve dictionary names up-front — drawer renders city/category by
    // their human-readable name (admin moderation tooling), and pinning the
    // assertions to the live dict keeps the test green when copy is tweaked.
    const cities = await fetchDictionary(request, "cities");
    const categories = await fetchDictionary(request, "categories");
    const cityName = lookupOrThrow(cities, "almaty", "cities");
    const beautyName = lookupOrThrow(categories, "beauty", "categories");
    const otherName = lookupOrThrow(categories, "other", "categories");

    const uuid = randomUUID();
    const igHandle = `aidana_${uuid.slice(0, 8)}`;
    const ttHandle = `tt_${uuid.slice(0, 8)}`;
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Кузнецова`,
      firstName: "Айгерим",
      middleName: "Олеговна",
      city: "almaty",
      categories: ["beauty", "other"],
      categoryOtherText: "Тест-ниша",
      socials: [
        { platform: "instagram", handle: igHandle },
        { platform: "tiktok", handle: ttHandle },
      ],
    });
    cleanupStack.push(application.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
    await expect(
      page.getByTestId("creator-applications-verification-page"),
    ).toBeVisible();

    // Open the filter popover before searching — the moderator's workflow is
    // "open filters → narrow down by uuid". Close it via the toggle (NOT
    // Escape — Chromium clears <input type="search"> on Escape, wiping the
    // just-typed uuid) before clicking the row, otherwise the popover
    // overlays the top rows and intercepts pointer events.
    await page.getByTestId("filters-toggle").click();
    await expect(page.getByTestId("filters-popover")).toBeVisible();
    await page.getByTestId("filters-search").fill(uuid);
    await page.getByTestId("filters-toggle").click();
    await expect(page.getByTestId("filters-popover")).toHaveCount(0);

    const row = page.getByTestId(`row-${application.applicationId}`);
    await expect(row).toBeVisible();
    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(1);

    // Click the index cell — the socials column intentionally stops
    // propagation so its <a> links don't open the drawer; targeting the
    // first <td> sidesteps that without bypassing the row-level handler.
    await row.locator("td").first().click();

    await expect(page).toHaveURL(
      new RegExp(`[?&]id=${application.applicationId}\\b`),
    );
    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    // Header — full name = last + first + middle joined by spaces. Mirrors
    // buildFullName in ApplicationDrawer.tsx (filter(Boolean)) so a future
    // null/empty middleName tightens the same expectation without rewrite.
    const fullName = buildExpectedFullName(application);
    await expect(drawer.getByTestId("drawer-full-name")).toHaveText(fullName);

    // Timeline — submitted-at line in ru locale: "Подана: 2 мая 2026 г. в 17:21"
    // (Intl ru-RU uses "г. в HH:MM" — letter "в" separates the date and time).
    await expect(drawer.getByTestId("application-timeline")).toContainText(
      /Подана: \d{1,2} [а-я]+ \d{4} г\. в \d{2}:\d{2}/,
    );

    // Birth date — "dd.mm.yyyy · N {год|года|лет}" with proper Russian plural
    const dd = String(application.birthDate.getUTCDate()).padStart(2, "0");
    const mm = String(application.birthDate.getUTCMonth() + 1).padStart(2, "0");
    const yyyy = application.birthDate.getUTCFullYear();
    const age = ageInYearsUTC(application.birthDate);
    await expect(drawer.getByTestId("drawer-birth-date")).toHaveText(
      `${dd}.${mm}.${yyyy} · ${age} ${pluralYears(age)}`,
    );

    // IIN
    await expect(drawer.getByTestId("drawer-iin")).toHaveText(application.iin);

    // Phone — visible text + tel: deep-link
    const phoneLink = drawer.getByTestId("application-phone");
    await expect(phoneLink).toHaveText(application.phone);
    await expect(phoneLink).toHaveAttribute("href", `tel:${application.phone}`);

    // City — name resolved from the live cities dictionary
    await expect(drawer.getByTestId("drawer-city")).toHaveText(cityName);

    // Categories — one chip per dict name + one italic chip "Другое: ${text}".
    // The "Другое:" prefix comes from the creatorApplications i18n bundle
    // (drawer.categoryOther) — coincidentally same as the "other" dict name
    // today, but tested independently of it via separate testid.
    await expect(drawer.getByTestId("drawer-category-beauty")).toHaveText(
      beautyName,
    );
    await expect(drawer.getByTestId("drawer-category-other")).toHaveText(
      otherName,
    );
    if (application.categoryOtherText !== null) {
      await expect(
        drawer.getByTestId("drawer-category-other-text"),
      ).toHaveText(`Другое: ${application.categoryOtherText}`);
    }

    // Socials — handle text inside the platform-keyed link
    await expect(drawer.getByTestId("social-instagram")).toContainText(igHandle);
    await expect(drawer.getByTestId("social-tiktok")).toContainText(ttHandle);

    // Telegram — not linked branch with copy-message button present, linked
    // branch absent (so a regression that flips the conditional is caught)
    await expect(drawer.getByTestId("drawer-telegram-not-linked")).toBeVisible();
    await expect(drawer.getByTestId("drawer-copy-bot-message")).toBeVisible();
    await expect(drawer.getByTestId("drawer-telegram-linked")).toHaveCount(0);

    // Close — drawer detaches and ?id= clears from URL
    await drawer.getByTestId("drawer-close").click();
    await expect(page.getByTestId("drawer")).toHaveCount(0);
    await expect(page).not.toHaveURL(/[?&]id=/);
  });

  test("Creator without middleName — drawer header collapses to two-word form", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    // middleName=null tells the helper to omit the field from the request
    // body. Backend's trimOptional then stores nil; drawer's buildFullName
    // filter(Boolean) collapses to "<lastName> <firstName>" with no padding.
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Сатпаева`,
      firstName: "Анель",
      middleName: null,
    });
    cleanupStack.push(application.cleanup);
    expect(application.middleName).toBeNull();

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
    const fullName = buildExpectedFullName(application);
    expect(fullName).toBe(`${application.lastName} ${application.firstName}`);
    await expect(drawer.getByTestId("drawer-full-name")).toHaveText(fullName);
  });

  test("Drawer prev/next — keyboard + buttons traverse newest-first list", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    // Sequential POSTs — created_at desc means the last seeded is the first row.
    // The 10ms sleep inside seedCreatorApplication keeps timestamps strictly
    // monotonic even when the runner clock has microsecond-precision ties.
    const a = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-A`,
    });
    cleanupStack.push(a.cleanup);
    const b = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-B`,
    });
    cleanupStack.push(b.cleanup);
    const c = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-C`,
    });
    cleanupStack.push(c.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
    await page.getByTestId("filters-search").fill(uuid);

    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(3);

    // Newest first — c, b, a. Click the index cell of the first row (c).
    // The socials column stops propagation (so its <a> links don't open the
    // drawer), so we always target td:first.
    await page
      .getByTestId(`row-${c.applicationId}`)
      .locator("td")
      .first()
      .click();
    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    const headerVisible = (app: SeededCreatorApplication) =>
      expect(drawer.getByTestId("drawer-full-name")).toHaveText(
        buildExpectedFullName(app),
      );

    await headerVisible(c);
    await expect(drawer.getByTestId("drawer-prev")).toBeDisabled();
    await expect(drawer.getByTestId("drawer-next")).toBeEnabled();

    // Button click — c → b
    await drawer.getByTestId("drawer-next").click();
    await headerVisible(b);

    // Keyboard — b → a
    await page.keyboard.press("ArrowRight");
    await headerVisible(a);
    await expect(drawer.getByTestId("drawer-next")).toBeDisabled();

    // Prev button — a → b
    await drawer.getByTestId("drawer-prev").click();
    await headerVisible(b);

    // Escape — close
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("drawer")).toHaveCount(0);
  });

  test("Filter telegramLinked — three branches each leave only matching rows", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    const uuid = randomUUID();
    const noTg = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-noTG`,
    });
    cleanupStack.push(noTg.cleanup);
    const withTg = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-withTG`,
    });
    cleanupStack.push(withTg.cleanup);
    await linkTelegramToApplication(request, API_URL, withTg.applicationId);

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
    await page.getByTestId("filters-search").fill(uuid);

    const tableRows = page
      .getByTestId("applications-table")
      .locator("tbody tr");

    // Default = "any" — both rows
    await expect(tableRows).toHaveCount(2);

    // Open filter popover so the telegram-linked segment is reachable
    await page.getByTestId("filters-toggle").click();
    await expect(page.getByTestId("filters-popover")).toBeVisible();

    // Branch "true" — only the linked one
    await page.getByTestId("filter-telegram-linked-true").click();
    await expect(tableRows).toHaveCount(1);
    await expect(page.getByTestId(`row-${withTg.applicationId}`)).toBeVisible();
    await expect(page.getByTestId(`row-${noTg.applicationId}`)).toHaveCount(0);

    // Branch "false" — only the un-linked one
    await page.getByTestId("filter-telegram-linked-false").click();
    await expect(tableRows).toHaveCount(1);
    await expect(page.getByTestId(`row-${noTg.applicationId}`)).toBeVisible();
    await expect(page.getByTestId(`row-${withTg.applicationId}`)).toHaveCount(0);

    // Back to "any" — both
    await page.getByTestId("filter-telegram-linked-any").click();
    await expect(tableRows).toHaveCount(2);
  });

  test("Empty state — random uuid in search yields empty-message and no table", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page
      .getByTestId("nav-link-creator-applications/verification")
      .click();
    await page.getByTestId("filters-search").fill(randomUUID());

    await expect(page.getByTestId("applications-table-empty")).toBeVisible();
    await expect(page.getByTestId("applications-table")).toHaveCount(0);
  });

  test("RoleGuard — brand_manager has no nav link and is redirected from /verification", async ({
    page,
    request,
  }) => {
    const bm = await seedBrandManager(request, API_URL);
    cleanupStack.push(bm.cleanup);

    await loginAs(page, bm.email, bm.password);
    await expect(page).toHaveURL("/");
    // Anchor the absence assertion to a positive render — count(0) on a blank
    // page would pass for the wrong reason; checking sidebar first turns it
    // into "sidebar is rendered AND verification link is missing from it".
    await expect(page.getByTestId("sidebar")).toBeVisible();
    await expect(
      page.getByTestId("nav-link-creator-applications/verification"),
    ).toHaveCount(0);

    // Direct goto on the protected route — RoleGuard redirects to ROUTES.DASHBOARD
    await page.goto("/creator-applications/verification", {
      waitUntil: "domcontentloaded",
    });
    await expect(page).toHaveURL("/");
    await expect(page.getByTestId("dashboard-page")).toBeVisible();
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

// buildExpectedFullName mirrors buildFullName in ApplicationDrawer.tsx. Kept
// in one place so empty/null middleName cases stay aligned with what the
// drawer actually renders without a per-test re-derivation.
function buildExpectedFullName(app: SeededCreatorApplication): string {
  return [app.lastName, app.firstName, app.middleName]
    .filter(Boolean)
    .join(" ");
}

// withTimeout races the given promise against a sleep so cleanup of a hung
// backend call surfaces in seconds, not after Playwright's 60s ceiling.
async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  label: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label} timed out after ${ms}ms`)), ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

// ageInYearsUTC mirrors filters.calcAge in the web bundle — with browser
// timezoneId pinned to UTC above, getFullYear() in the browser equals
// getUTCFullYear() here, so both sides agree.
function ageInYearsUTC(birth: Date): number {
  const now = new Date();
  let age = now.getUTCFullYear() - birth.getUTCFullYear();
  const m = now.getUTCMonth() - birth.getUTCMonth();
  if (m < 0 || (m === 0 && now.getUTCDate() < birth.getUTCDate())) age--;
  return age;
}

// pluralYears mirrors the same-named helper in ApplicationDrawer.tsx.
function pluralYears(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "год";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return "года";
  return "лет";
}

// fetchDictionary loads the public dictionary endpoint and returns a code →
// name map. Used to assert the drawer's rendered text against the live
// dictionary instead of hardcoded copy that drifts as content is tuned.
async function fetchDictionary(
  request: APIRequestContext,
  type: "cities" | "categories",
): Promise<Map<string, string>> {
  const resp = await request.get(`${API_URL}/dictionaries/${type}`);
  if (!resp.ok()) {
    throw new Error(`fetchDictionary ${type}: ${resp.status()}`);
  }
  const body = (await resp.json()) as {
    data: { items: { code: string; name: string }[] };
  };
  return new Map(body.data.items.map((i) => [i.code, i.name]));
}

function lookupOrThrow(
  map: Map<string, string>,
  code: string,
  dictName: string,
): string {
  const value = map.get(code);
  if (!value) throw new Error(`${dictName} dict missing code "${code}"`);
  return value;
}
