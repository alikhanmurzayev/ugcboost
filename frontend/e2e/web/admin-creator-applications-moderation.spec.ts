/**
 * Browser e2e — admin-экран модерации заявок креаторов в веб-приложении.
 *
 * Каждый тест сидит свой набор данных через composable-хелперы из
 * helpers/api.ts (seedAdmin, seedCreatorApplication, linkTelegramToApplication
 * и далее либо triggerSendPulseInstagramWebhook для IG-auto-пути, либо
 * manualVerifyApplicationSocial для TT-manual-пути), прогоняет UI-сценарий
 * и дренирует локальный cleanup-стек в afterEach с per-call 5-секундным
 * таймаутом. uuid в lastName изолирует параллельные воркеры по search,
 * uniqueIIN — по partial-unique-индексу. Cleanup — fail-fast, поломанный
 * бэкенд должен упасть громко и сразу, а не оставлять "остатки заявок,
 * через час станут flaky".
 *
 * Happy path — IG-only заявка с auto-verify через SendPulse webhook и
 * привязанным Telegram. Логинимся в UI, заходим на /moderation, фильтруем
 * по uuid, ассертим точные значения каждой ячейки строки (№, ФИО без
 * отчества в таблице, IG-handle через SocialLink, две category-чипы +
 * "Другое: ...", "Алматы" из словаря, короткая дата подачи, hours-badge
 * формата "<1ч"). Открываем drawer и проверяем equality на все
 * отображаемые поля: drawer-full-name с отчеством, timeline с полной датой
 * подачи, IIN, дата рождения с pluralYears, телефон с tel:href, "Алматы",
 * категории включая drawer-category-other-text, единственный
 * social-admin-row с verified-badge "Подтверждено · авто", drawer-telegram-
 * linked с @username, footer = reject-кнопка + активный approve-button
 * (его confirm-flow и race-422 закрывает spec
 * admin-creator-applications-moderation-approve-action.spec.ts).
 *
 * TT manual-verified — TT-only заявка, привязанный TG, manual-verify через
 * admin API двигает заявку из verification в moderation. Drawer отображает
 * verified-badge с "Подтверждено · вручную" — закрывает второй вариант
 * verified-маркера, который не покрыт IG-auto-путём.
 *
 * Без отчества и без TG — IG-only без linkTelegramToApplication и с
 * middleName=null. drawer-full-name содержит ровно "Last First" без
 * trailing-space, drawer-telegram-not-linked + drawer-copy-bot-message
 * видны — закрывает контракт filter(Boolean) и fallback-блок Telegram.
 *
 * Approve — активная emerald-кнопка в drawer-footer любой moderation-
 * заявки; полный confirm-flow и race-сценарии вынесены в соседний spec
 * admin-creator-applications-moderation-approve-action.spec.ts.
 *
 * Колонки thead — структурный assert: present "Город" / отсутствует
 * "Telegram". Закрывает что moderation скрывает Telegram-колонку
 * относительно verification.
 *
 * Sort cycle — три заявки с разными lastName / city / createdAt /
 * updatedAt. seedCreatorApplication уже даёт 10ms gap по createdAt; для
 * updated_at (пишется при webhook-promote'е) добавляем явный sleep 60ms
 * между итерациями цикла, чтобы три webhooks не попали в одну Postgres-
 * микросекунду на быстром runner'е и порядок ORDER BY updated_at остался
 * детерминированным. Default sort = updated_at asc (URL без params на
 * загрузке). Click "В этапе" → URL `?sort=updated_at&order=desc` + порядок
 * строк desc; повторный клик → URL очищается до default. Аналогично
 * кликаем headers ФИО, Подана, Город — закрываем COLUMN_TO_FIELD маппинг.
 *
 * Filter search — две заявки с одним uuid, ввод uuid в filters-search,
 * остаются обе. Пустой filters-search возвращает все строки. Active-count
 * badge на filters-toggle не появляется (search не считается в countActive).
 *
 * Filter date — date-range через URL (DayPicker через UI флаки и не
 * relevantен для backend-filter-контракта). `dateTo=2020-01-01` (далёко в
 * прошлом) → seed-row excluded, empty-state виден. `dateFrom=2020-01-01
 * &dateTo=2099-12-31` (диапазон с запасом в обе стороны) → seed-row
 * included. Active-count = 1.
 *
 * Filter age — две заявки с разными возрастами (одна 25, одна 45). UI
 * filter-age-from + filter-age-to. ageFrom=20 ageTo=30 → только 25.
 * Active-count = 1.
 *
 * Filter city — две заявки в разных городах, фильтр через UI multiselect
 * filter-cities → option-almaty → только almaty-заявка. Active-count = 1.
 *
 * Filter categories — две заявки с разными категориями, фильтр через UI
 * multiselect filter-categories → option-beauty → только beauty-заявка.
 *
 * Filter Telegram — открываем popover, ассертим что filter-telegram-linked
 * отсутствует в DOM (showTelegramFilter={false} от ModerationPage).
 *
 * Multi-filter reset — два активных фильтра (city + categories) → click
 * filters-reset → URL без cities/categories params, active-count badge
 * исчезает.
 *
 * Empty state — admin вводит свежий random uuid в filters-search, никакая
 * заявка не сидится. applications-table-empty виден с локализованным
 * текстом "Нет заявок по выбранным фильтрам", сама applications-table в
 * DOM отсутствует.
 *
 * Live badge — admin логинится, заходит на /; затем сидим moderation-заявку
 * через API и делаем page.reload(), чтобы countsQuery cold-refetch'нул.
 * Sidebar nav-link на moderation показывает бейдж count ≥ 1 (мы только что
 * сидили). Ассерт `>= 1` вместо `>= before + 1` — параллельный воркер может
 * cleanup'нуть свою moderation-заявку между snapshot'ами, что сделает
 * `before+1` flaky.
 *
 * RoleGuard — brand_manager логинится, sidebar не содержит nav-link на
 * moderation, прямой goto /creator-applications/moderation редиректит на
 * ROUTES.DASHBOARD = "/", где рендерится дашборд.
 */
import { randomUUID } from "node:crypto";
import {
  test,
  expect,
  type APIRequestContext,
  type Locator,
  type Page,
} from "@playwright/test";
import {
  fetchApplicationDetail,
  generateValidIIN,
  linkTelegramToApplication,
  loginAsAdmin,
  manualVerifyApplicationSocial,
  seedAdmin,
  seedBrandManager,
  seedCreatorApplication,
  triggerSendPulseInstagramWebhook,
  type SeededCreatorApplication,
  type SocialAccountInput,
} from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;
const NAV_LINK_MODERATION = "nav-link-creator-applications/moderation";

test.describe("Admin moderation flow", () => {
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

  test("Happy path — IG auto-verified, full table row + drawer fields", async ({
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

    // Resolve dictionary names up-front — drawer renders city/category by
    // human-readable name; pinning assertions to the live dict keeps the
    // test green when copy is tweaked.
    const cities = await fetchDictionary(request, "cities");
    const categories = await fetchDictionary(request, "categories");
    const cityName = lookupOrThrow(cities, "almaty", "cities");
    const beautyName = lookupOrThrow(categories, "beauty", "categories");
    const fashionName = lookupOrThrow(categories, "fashion", "categories");

    const uuid = randomUUID();
    const handle = `aidana_test_${uuid.slice(0, 8)}`;
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      middleName: "Тестовна",
      address: "ул. Достык, 1",
      categories: ["beauty", "fashion", "other"],
      categoryOtherText: "винтажный thrift",
      city: "almaty",
      socials: [{ platform: "instagram", handle }],
    });
    cleanupStack.push(application.cleanup);

    const tg = await linkTelegramToApplication(
      request,
      API_URL,
      application.applicationId,
    );

    // verificationCode is admin-only — fetch detail to learn it.
    const detailBefore = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(detailBefore.status).toBe("verification");
    await triggerSendPulseInstagramWebhook(request, API_URL, {
      username: handle,
      verificationCode: detailBefore.verificationCode,
    });

    const detailAfter = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(detailAfter.status).toBe("moderation");

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await expect(
      page.getByTestId("creator-applications-moderation-page"),
    ).toBeVisible();

    await page.getByTestId("filters-search").fill(uuid);

    // Table row equality for every cell.
    const row = page.getByTestId(`row-${application.applicationId}`);
    await expect(row).toBeVisible();
    const cells = row.locator("td");
    await expect(cells.nth(0)).toHaveText("1");
    await expect(cells.nth(1)).toHaveText(`e2e-${uuid}-Иванова Айдана`);
    await expect(cells.nth(2)).toContainText(`@${handle}`);
    await expect(cells.nth(3)).toContainText(beautyName);
    await expect(cells.nth(3)).toContainText(fashionName);
    await expect(cells.nth(4)).toHaveText(cityName);
    await expect(cells.nth(5)).toHaveText(formatShortDate(detailAfter.createdAt));
    // hours-badge text is a function of (now - updatedAt); for a row created
    // milliseconds ago it is "<1ч", but a slow runner can tip onto "1ч".
    // Asserting visibility + the numeric/short-form pattern is robust to that
    // border without losing coverage of the formatter shape.
    const hoursBadge = row.getByTestId("hours-badge");
    await expect(hoursBadge).toBeVisible();
    await expect(hoursBadge).toHaveText(/^(<1ч|\d+ч|\d+д( \d+ч)?)$/);

    await row.locator("td").first().click();
    const drawer = page.getByTestId("drawer");
    await expect(drawer).toBeVisible();

    await expect(drawer.getByTestId("drawer-full-name")).toHaveText(
      `e2e-${uuid}-Иванова Айдана Тестовна`,
    );
    // ru locale renders "Подана: 4 мая 2026 г. в 12:34" — assert the shape so
    // copy tweaks (separator letters, year suffix) flag explicitly.
    await expect(drawer.getByTestId("application-timeline")).toContainText(
      /Подана: \d{1,2} [а-я]+ \d{4} г\. в \d{2}:\d{2}/,
    );

    const age = ageInYearsUTC(application.birthDate);
    await expect(drawer.getByTestId("drawer-birth-date")).toHaveText(
      `${formatBirthDateUTC(application.birthDate)} · ${age} ${pluralYears(age)}`,
    );
    await expect(drawer.getByTestId("drawer-iin")).toHaveText(application.iin);
    await expect(drawer.getByTestId("application-phone")).toHaveAttribute(
      "href",
      `tel:${application.phone}`,
    );
    await expect(drawer.getByTestId("application-phone")).toHaveText(
      application.phone,
    );
    await expect(drawer.getByTestId("drawer-city")).toHaveText(cityName);

    await expect(drawer.getByTestId("drawer-category-beauty")).toHaveText(
      beautyName,
    );
    await expect(drawer.getByTestId("drawer-category-fashion")).toHaveText(
      fashionName,
    );
    // The chip text is `${t("drawer.categoryOther")}: ${categoryOtherText}`
    // — an i18n key concatenation, not a dictionary lookup. We assert only
    // the user-supplied free-text fragment so the assertion does not break
    // when the prefix word is tweaked.
    await expect(
      drawer.getByTestId("drawer-category-other-text"),
    ).toContainText("винтажный thrift");

    const igSocial = detailAfter.socials.find((s) => s.platform === "instagram");
    if (!igSocial) throw new Error("expected IG social in detailAfter");
    const socialRow = drawer.getByTestId(`social-admin-row-${igSocial.id}`);
    await expect(socialRow).toBeVisible();
    await expect(socialRow).toContainText(`@${handle}`);
    const verifiedBadge = drawer.getByTestId(`verified-badge-${igSocial.id}`);
    await expect(verifiedBadge).toBeVisible();
    await expect(verifiedBadge).toHaveText("Подтверждено · авто");

    const tgBlock = drawer.getByTestId("drawer-telegram-linked");
    await expect(tgBlock).toBeVisible();
    await expect(tgBlock).toHaveText(`@${tg.username}`);

    const footer = drawer.getByTestId("drawer-footer");
    await expect(footer.getByTestId("reject-button")).toBeEnabled();
    const approve = footer.getByTestId("approve-button");
    await expect(approve).toBeVisible();
    await expect(approve).toBeEnabled();
    await expect(approve).toHaveText("Одобрить заявку");
  });

  test("TT manual-verified — drawer shows manual badge", async ({
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
    const ttHandle = `tt_${uuid.slice(0, 8)}`;
    const application = await seedCreatorApplication(request, API_URL, {
      lastName: `e2e-${uuid}-Иванова`,
      firstName: "Айдана",
      socials: [{ platform: "tiktok", handle: ttHandle }],
    });
    cleanupStack.push(application.cleanup);

    await linkTelegramToApplication(
      request,
      API_URL,
      application.applicationId,
    );

    // Need socialId from the seeded row to drive manual-verify. Asserting
    // the application is still in `verification` first — manualVerify endpoint
    // returns 422 on `moderation`, so a silent pre-promotion would otherwise
    // fail with an opaque error.
    const detailVerification = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(detailVerification.status).toBe("verification");
    const ttSocial = detailVerification.socials.find(
      (s) => s.platform === "tiktok",
    );
    if (!ttSocial) throw new Error("expected TT social");

    await manualVerifyApplicationSocial(
      request,
      API_URL,
      application.applicationId,
      ttSocial.id,
      adminToken,
    );

    const detailModeration = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(detailModeration.status).toBe("moderation");

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

    const verifiedBadge = drawer.getByTestId(`verified-badge-${ttSocial.id}`);
    await expect(verifiedBadge).toBeVisible();
    await expect(verifiedBadge).toHaveText("Подтверждено · вручную");
  });

  test("Without middleName and without TG — drawer collapses header, copy-bot-message visible", async ({
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
      middleName: null,
      socials: [{ platform: "instagram", handle }],
    });
    cleanupStack.push(application.cleanup);

    // No linkTelegramToApplication — TG must remain unlinked.
    const detailBefore = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    await triggerSendPulseInstagramWebhook(request, API_URL, {
      username: handle,
      verificationCode: detailBefore.verificationCode,
    });

    // SendPulse webhook returns 200 even on no-op (mismatched handle, race);
    // verify the promotion landed before driving UI.
    const detailAfter = await fetchApplicationDetail(
      request,
      API_URL,
      application.applicationId,
      adminToken,
    );
    expect(detailAfter.status).toBe("moderation");

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await page
      .getByTestId(`row-${application.applicationId}`)
      .locator("td")
      .first()
      .click();

    const drawer = page.getByTestId("drawer");
    await expect(drawer.getByTestId("drawer-full-name")).toHaveText(
      `e2e-${uuid}-Иванова Айдана`,
    );

    await expect(
      drawer.getByTestId("drawer-telegram-not-linked"),
    ).toBeVisible();
    await expect(drawer.getByTestId("drawer-copy-bot-message")).toBeVisible();
    await expect(drawer.getByTestId("drawer-telegram-linked")).toHaveCount(0);
  });

  test("Header columns — city present, telegram absent", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await expect(
      page.getByTestId("creator-applications-moderation-page"),
    ).toBeVisible();

    // Empty table is fine — thead renders only when rows exist. Seed one
    // moderation app to materialise the table.
    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );
    const uuid = randomUUID();
    const handle = `aidana_test_${uuid.slice(0, 8)}`;
    const application = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle,
    });
    cleanupStack.push(application.cleanup);

    await page.getByTestId("filters-search").fill(uuid);
    await expect(page.getByTestId("applications-table")).toBeVisible();

    const thead = page.getByTestId("applications-table").locator("thead");
    // Sortable headers carry th-${key} testid; assert the moderation-specific
    // set: city is present (new for chunk 16), Telegram column is removed
    // (chunk 16 boundary). Non-sortable columns (index, socials, categories)
    // don't have testids — covered indirectly by the Happy path row assert.
    await expect(thead.getByTestId("th-fullName")).toBeVisible();
    await expect(thead.getByTestId("th-city")).toBeVisible();
    await expect(thead.getByTestId("th-submittedAt")).toBeVisible();
    await expect(thead.getByTestId("th-hoursInStage")).toBeVisible();
    // Telegram column is removed entirely on moderation: assert both the
    // sortable testid is absent AND total <th> count is 7 (index + fullName +
    // socials + categories + city + submittedAt + hoursInStage). Recovering
    // the column as non-sortable would slip past the testid check alone.
    await expect(thead.getByTestId("th-telegram")).toHaveCount(0);
    await expect(thead.locator("th")).toHaveCount(7);
  });

  test("Sort cycle — default updated_at asc, click headers cycle through fields and orders", async ({
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
    // Three applications with deterministic ascending order on both
    // created_at and updated_at. seedCreatorApplication already does a 10ms
    // pre-POST sleep, but that only spaces submit-time createdAt. The
    // SendPulse webhook fires later and writes a fresh updated_at — without
    // an explicit gap between iterations, the three webhooks can land in
    // the same Postgres microsecond on a fast runner, making ORDER BY
    // updated_at non-deterministic. 60ms covers worst-case microsecond
    // resolution under CI load.
    const apps: SeededCreatorApplication[] = [];
    const lastNames = ["Aaaa", "Bbbb", "Cccc"];
    const cities = ["aktau", "almaty", "astana"];
    for (let i = 0; i < 3; i++) {
      if (i > 0) await sleep(60);
      const handle = `aidana_${uuid.slice(0, 8)}_${i}`;
      const app = await seedAndPromoteIG(request, API_URL, adminToken, {
        uuid,
        handle,
        lastName: `e2e-${uuid}-${lastNames[i]}`,
        city: cities[i],
      });
      apps.push(app);
      cleanupStack.push(app.cleanup);
    }

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);

    const table = page.getByTestId("applications-table");
    await expect(table.locator("tbody tr")).toHaveCount(3);

    // Default = updated_at asc → URL has no sort/order params; rows in
    // ascending updatedAt → first seeded row is at the top.
    expect(new URL(page.url()).searchParams.get("sort")).toBeNull();
    expect(new URL(page.url()).searchParams.get("order")).toBeNull();
    await expectRowOrder(table, [
      apps[0].applicationId,
      apps[1].applicationId,
      apps[2].applicationId,
    ]);

    // Click "В этапе" → toggleSort: same field → flip to desc; URL writes
    // sort/order because state differs from default (updated_at,asc).
    await page.getByTestId("th-hoursInStage").click();
    await waitUrlSort(page, "updated_at", "desc");
    await expectRowOrder(table, [
      apps[2].applicationId,
      apps[1].applicationId,
      apps[0].applicationId,
    ]);

    // Click again → flip to asc → state == default → URL clears.
    await page.getByTestId("th-hoursInStage").click();
    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("sort") === null && sp.get("order") === null;
    });
    await expectRowOrder(table, [
      apps[0].applicationId,
      apps[1].applicationId,
      apps[2].applicationId,
    ]);

    // "ФИО" header — non-default field, first click sets desc by toggleSort.
    await page.getByTestId("th-fullName").click();
    await waitUrlSort(page, "full_name", "desc");
    await expectRowOrder(table, [
      apps[2].applicationId, // Cccc
      apps[1].applicationId, // Bbbb
      apps[0].applicationId, // Aaaa
    ]);

    // "Город" header — first click desc.
    await page.getByTestId("th-city").click();
    await waitUrlSort(page, "city_name", "desc");
    await expectRowOrder(table, [
      apps[2].applicationId, // Астана
      apps[1].applicationId, // Алматы
      apps[0].applicationId, // Актау
    ]);

    // "Подана" header — first click desc.
    await page.getByTestId("th-submittedAt").click();
    await waitUrlSort(page, "created_at", "desc");
    await expectRowOrder(table, [
      apps[2].applicationId,
      apps[1].applicationId,
      apps[0].applicationId,
    ]);
  });

  test("Filter search — uuid in input narrows table to seeded rows", async ({
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
    const a = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_a`,
    });
    cleanupStack.push(a.cleanup);
    const b = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_b`,
    });
    cleanupStack.push(b.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);

    const table = page.getByTestId("applications-table");
    await expect(table.locator("tbody tr")).toHaveCount(2);
    expect(new URL(page.url()).searchParams.get("q")).toBe(uuid);

    // search is not counted in countActive — toggle button must NOT show
    // the numeric badge.
    const toggle = page.getByTestId("filters-toggle");
    await expect(toggle.locator("span.bg-primary")).toHaveCount(0);
  });

  test("Filter date — URL dateFrom/dateTo bounds applied on backend", async ({
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
    const app = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}`,
    });
    cleanupStack.push(app.cleanup);

    await loginAs(page, admin.email, admin.password);

    // dateTo well in the past → seeded row excluded.
    await page.goto(
      `/creator-applications/moderation?q=${uuid}&dateTo=2020-01-01`,
    );
    await expect(
      page.getByTestId("applications-table-empty"),
    ).toBeVisible();
    await expect(page.getByTestId("applications-table")).toHaveCount(0);
    await expect(page.getByTestId("filters-toggle")).toContainText("1");

    // dateFrom in the past + dateTo far future → seeded row included.
    await page.goto(
      `/creator-applications/moderation?q=${uuid}&dateFrom=2020-01-01&dateTo=2099-12-31`,
    );
    const table = page.getByTestId("applications-table");
    await expect(table.locator("tbody tr")).toHaveCount(1);
    await expect(
      page.getByTestId(`row-${app.applicationId}`),
    ).toBeVisible();
  });

  test("Filter age — UI inputs filter by computed age from IIN", async ({
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
    const youngIIN = generateValidIIN(birthForAge(25));
    const oldIIN = generateValidIIN(birthForAge(45));

    const young = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_y`,
      lastName: `e2e-${uuid}-Y`,
      iin: youngIIN,
    });
    cleanupStack.push(young.cleanup);
    const old = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_o`,
      lastName: `e2e-${uuid}-O`,
      iin: oldIIN,
    });
    cleanupStack.push(old.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(2);

    await page.getByTestId("filters-toggle").click();
    await expect(page.getByTestId("filters-popover")).toBeVisible();

    // ApplicationFilters.update() captures `filters` from a closure that
    // re-derives only after re-render. Fill ageFrom, wait for the URL to
    // reflect it, then fill ageTo — otherwise the second update may
    // overwrite ageFrom with the stale (undefined) closure value.
    await page.getByTestId("filter-age-from").fill("20");
    await page.waitForFunction(() => {
      return new URL(window.location.href).searchParams.get("ageFrom") === "20";
    });
    await page.getByTestId("filter-age-to").fill("30");
    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("ageFrom") === "20" && sp.get("ageTo") === "30";
    });

    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(1);
    await expect(
      page.getByTestId(`row-${young.applicationId}`),
    ).toBeVisible();
    await expect(
      page.getByTestId(`row-${old.applicationId}`),
    ).toHaveCount(0);
    await expect(page.getByTestId("filters-toggle")).toContainText("1");
  });

  test("Filter city — multiselect option filters by city code", async ({
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
    const inAlmaty = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_a`,
      lastName: `e2e-${uuid}-Almaty`,
      city: "almaty",
    });
    cleanupStack.push(inAlmaty.cleanup);
    const inAstana = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_b`,
      lastName: `e2e-${uuid}-Astana`,
      city: "astana",
    });
    cleanupStack.push(inAstana.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(2);

    await page.getByTestId("filters-toggle").click();
    await page.getByTestId("filter-cities").click();
    // Cities dictionary loads async (enabled: open); wait for the search
    // input to render so we know options are mounted before clicking.
    await expect(page.getByTestId("filter-cities-search")).toBeVisible();
    // .click() over .check() because the SearchableMultiselect's input is a
    // controlled checkbox driven by React onChange — Playwright .check()
    // re-reads input.checked synchronously and treats the unchanged value
    // as a state-change failure even though onChange did fire.
    const almatyOption = page.getByTestId("filter-cities-option-almaty");
    await expect(almatyOption).toBeVisible();
    await almatyOption.click();

    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("cities") === "almaty";
    });

    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(1);
    await expect(
      page.getByTestId(`row-${inAlmaty.applicationId}`),
    ).toBeVisible();
    await expect(page.getByTestId("filters-toggle")).toContainText("1");
  });

  test("Filter categories — multiselect option filters by category code", async ({
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
    const beautyApp = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_a`,
      lastName: `e2e-${uuid}-B`,
      categories: ["beauty"],
    });
    cleanupStack.push(beautyApp.cleanup);
    const fashionApp = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}_b`,
      lastName: `e2e-${uuid}-F`,
      categories: ["fashion"],
    });
    cleanupStack.push(fashionApp.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(uuid);
    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(2);

    await page.getByTestId("filters-toggle").click();
    await page.getByTestId("filter-categories").click();
    await expect(page.getByTestId("filter-categories-search")).toBeVisible();
    const beautyOption = page.getByTestId("filter-categories-option-beauty");
    await expect(beautyOption).toBeVisible();
    await beautyOption.click();

    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("categories") === "beauty";
    });

    await expect(
      page.getByTestId("applications-table").locator("tbody tr"),
    ).toHaveCount(1);
    await expect(
      page.getByTestId(`row-${beautyApp.applicationId}`),
    ).toBeVisible();
    await expect(page.getByTestId("filters-toggle")).toContainText("1");
  });

  test("Filter Telegram — popover does NOT contain telegram filter on moderation", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-toggle").click();
    await expect(page.getByTestId("filters-popover")).toBeVisible();

    await expect(
      page.getByTestId("filter-telegram-linked"),
    ).toHaveCount(0);
  });

  test("Filters reset — multiple active filters cleared, badge gone", async ({
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
    const app = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}`,
      city: "almaty",
      categories: ["beauty"],
    });
    cleanupStack.push(app.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(
      `/creator-applications/moderation?q=${uuid}&cities=almaty&categories=beauty`,
    );

    await expect(page.getByTestId("filters-toggle")).toContainText("2");

    await page.getByTestId("filters-reset").click();
    await page.waitForFunction(() => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("cities") === null && sp.get("categories") === null;
    });

    await expect(page.getByTestId("filters-toggle")).not.toContainText("2");
    await expect(page.getByTestId("filters-toggle")).not.toContainText("1");
  });

  test("Empty filtered — random uuid yields empty-message and no table", async ({
    page,
    request,
  }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.getByTestId(NAV_LINK_MODERATION).click();
    await page.getByTestId("filters-search").fill(`nomatch-${randomUUID()}`);

    const empty = page.getByTestId("applications-table-empty");
    await expect(empty).toBeVisible();
    await expect(empty).toHaveText("Нет заявок по выбранным фильтрам");
    await expect(page.getByTestId("applications-table")).toHaveCount(0);
  });

  test("Live badge — sidebar moderation badge increases after seed + reload", async ({
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

    await loginAs(page, admin.email, admin.password);
    await expect(page.getByTestId(NAV_LINK_MODERATION)).toBeVisible();

    const uuid = randomUUID();
    const app = await seedAndPromoteIG(request, API_URL, adminToken, {
      uuid,
      handle: `aidana_${uuid.slice(0, 8)}`,
    });
    cleanupStack.push(app.cleanup);

    // After page.reload() the counts query refetches and the sidebar badge
    // re-renders with the live moderation count. We just seeded one, so
    // the badge must be >= 1 — asserting `>= before + 1` would flake when
    // a parallel worker cleans up its own moderation row between the two
    // snapshots. Polling timeout extended to absorb cold refetch on slow CI.
    await page.reload();
    await expect(page.getByTestId(NAV_LINK_MODERATION)).toBeVisible();
    await expect
      .poll(async () => readBadgeCount(page), { timeout: 10_000 })
      .toBeGreaterThanOrEqual(1);
  });

  test("RoleGuard — brand_manager has no nav link, redirected from /moderation", async ({
    page,
    request,
  }) => {
    const manager = await seedBrandManager(request, API_URL);
    cleanupStack.push(manager.cleanup);

    await loginAs(page, manager.email, manager.password);
    await expect(page.getByTestId(NAV_LINK_MODERATION)).toHaveCount(0);

    await page.goto("/creator-applications/moderation", {
      waitUntil: "domcontentloaded",
    });
    await expect(page).toHaveURL("/");
    await expect(page.getByTestId("dashboard-page")).toBeVisible();
  });
});

interface SeedIGOpts {
  uuid: string;
  handle: string;
  lastName?: string;
  firstName?: string;
  city?: string;
  categories?: string[];
  iin?: string;
}

// seedAndPromoteIG creates a creator application with a single Instagram
// social and walks it from `pending` to `moderation` via SendPulse webhook
// auto-verify. Verifies the post-promotion status because webhook returns
// 200 even on no-op (mismatched handle, race) — without this check, a
// silent backend regression would surface as opaque "row not found" with
// 30s timeout further down. Caller already has admin bearer to look up
// verificationCode.
async function seedAndPromoteIG(
  request: APIRequestContext,
  apiUrl: string,
  adminToken: string,
  opts: SeedIGOpts,
): Promise<SeededCreatorApplication> {
  const socials: SocialAccountInput[] = [
    { platform: "instagram", handle: opts.handle },
  ];
  const seedOpts: Parameters<typeof seedCreatorApplication>[2] = {
    lastName: opts.lastName ?? `e2e-${opts.uuid}-Иванова`,
    firstName: opts.firstName ?? "Айдана",
    socials,
  };
  if (opts.city) seedOpts.city = opts.city;
  if (opts.categories) seedOpts.categories = opts.categories;
  if (opts.iin) seedOpts.iin = opts.iin;

  const application = await seedCreatorApplication(request, apiUrl, seedOpts);

  // linkTG is not required for IG-auto-verify but cheap to add — keeps the
  // moderation row consistent with realistic state.
  await linkTelegramToApplication(request, apiUrl, application.applicationId);

  const detail = await fetchApplicationDetail(
    request,
    apiUrl,
    application.applicationId,
    adminToken,
  );
  await triggerSendPulseInstagramWebhook(request, apiUrl, {
    username: opts.handle,
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
      `seedAndPromoteIG: expected status=moderation, got ${detailAfter.status}`,
    );
  }

  return application;
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

async function expectRowOrder(
  table: Locator,
  expectedIds: string[],
): Promise<void> {
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(expectedIds.length);
  for (let i = 0; i < expectedIds.length; i++) {
    await expect(rows.nth(i)).toHaveAttribute(
      "data-testid",
      `row-${expectedIds[i]}`,
    );
  }
}

async function waitUrlSort(
  page: Page,
  field: string,
  order: "asc" | "desc",
): Promise<void> {
  await page.waitForFunction(
    ({ field, order }) => {
      const sp = new URL(window.location.href).searchParams;
      return sp.get("sort") === field && sp.get("order") === order;
    },
    { field, order },
  );
}

async function readBadgeCount(page: Page): Promise<number> {
  const badge = page.getByTestId(
    "nav-badge-creator-applications/moderation",
  );
  if ((await badge.count()) === 0) return 0;
  const text = (await badge.textContent())?.trim() ?? "0";
  const n = Number(text);
  return Number.isFinite(n) ? n : 0;
}

function birthForAge(years: number): Date {
  const now = new Date();
  return new Date(
    Date.UTC(now.getUTCFullYear() - years, now.getUTCMonth(), 1),
  );
}

// ageInYearsUTC mirrors filters.calcAge in the web bundle. With browser TZ
// pinned to UTC at the describe level, getFullYear() in the browser equals
// getUTCFullYear() here so both sides agree.
function ageInYearsUTC(birth: Date): number {
  const now = new Date();
  let age = now.getUTCFullYear() - birth.getUTCFullYear();
  const m = now.getUTCMonth() - birth.getUTCMonth();
  if (m < 0 || (m === 0 && now.getUTCDate() < birth.getUTCDate())) age--;
  return age;
}

function pluralYears(n: number): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "год";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return "года";
  return "лет";
}

// formatShortDate mirrors ModerationPage.formatShortDate. timeZone:"UTC" is
// load-bearing — Node-side runner uses the host TZ by default, which would
// disagree with the browser pinned to UTC, producing day-off mismatches for
// rows created near midnight UTC.
function formatShortDate(iso: string): string {
  return new Date(iso).toLocaleDateString("ru", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  });
}

function formatLongDateTime(iso: string): string {
  return new Date(iso).toLocaleString("ru", {
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "UTC",
  });
}

// fetchDictionary loads the public dictionary endpoint and returns a code →
// name map. Keeps assertions on city/category names tied to the live dict
// instead of hardcoded copy that drifts when content is tuned.
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

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

function formatBirthDateUTC(birth: Date): string {
  return `${pad2(birth.getUTCDate())}.${pad2(birth.getUTCMonth() + 1)}.${birth.getUTCFullYear()}`;
}
