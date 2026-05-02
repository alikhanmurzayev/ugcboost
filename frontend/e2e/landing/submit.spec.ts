/**
 * Package landing — E2E тесты публичной формы заявки креатора (EFW-лендинг).
 *
 * Все тесты проходят полный browser-flow против настоящего бэкенда: dictionaries
 * подгружаются по-настоящему, форма сабмитится по-настоящему, success-screen или
 * server-error отображаются по-настоящему. Backend поднимается через webServer
 * playwright-конфига (локально) или через docker-compose (в `make
 * test-e2e-landing`). Cleanup делает afterAll: каждый созданный application_id
 * удаляется через POST /test/cleanup-entity, если E2E_CLEANUP !== "false" — при
 * false данные остаются в БД для дебага упавшего сценария.
 *
 * "Golden path" описывает путь, по которому ходит реальный креатор: открывает
 * страницу, ждёт, пока подтянутся справочники категорий и городов, заполняет
 * каждое required-поле валидными данными, чекает одну социалку, одну
 * не-other категорию и единственный чекбокс согласий, нажимает submit. После
 * успешного запроса лендинг показывает success-screen с CTA-кнопкой, ссылка
 * которой ведёт в Telegram-бот с application_id в start-параметре. Тест
 * проверяет, что справочники не пустые (форма не повисла на состоянии
 * "Загрузка…"), что URL CTA соответствует ожидаемому формату, и регистрирует
 * id для cleanup.
 *
 * "Other category" закрывает уникальную UI-ветку: при выборе категории
 * "Другое" появляется отдельный input для пользовательского описания ниши, и
 * без заполненного описания backend отвечает 422. Тест чекает only "other",
 * заполняет описание разумным текстом и убеждается, что заявка проходит как
 * обычная — success-screen появляется и показывает CTA. Backend при этом
 * сохраняет введённый текст в creator_applications.category_other_text — это
 * проверяется отдельно бэкендовым e2e (TestSubmitCreatorApplicationOther).
 *
 * "Server validation" проверяет, что серверная валидация попадает на UI:
 * креатор моложе MinCreatorAge получает от backend 422 UNDER_AGE, и форма
 * отображает текст ошибки в form-error блоке вместо перехода на success-screen.
 * IIN строится через underageIIN — он clock-independent, поэтому тест не
 * сломается через год. После ошибки кнопка submit снова enabled — креатор
 * может поправить данные и попробовать ещё раз без перезагрузки страницы.
 */
import { test, expect, type Page } from "@playwright/test";

import { cleanupCreatorApplication, underageIIN, uniqueIIN } from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";

// Application ids registered for cleanup. Kept module-level so afterAll can
// drain them in LIFO order regardless of which test created them.
// cleanupCreatorApplication itself honours E2E_CLEANUP=false and throws on
// any unexpected status — afterAll deliberately does not swallow errors so a
// stale row surfaces immediately instead of leaking into the next CI run.
const created: string[] = [];

test.afterAll(async ({ request }) => {
  while (created.length > 0) {
    const id = created.pop();
    if (id) await cleanupCreatorApplication(request, API_URL, id);
  }
});

// fillRequiredFields populates every required input on the form except the
// category checkboxes — those vary per scenario, so each test wires them up
// itself.
async function fillRequiredFields(page: Page, iin: string): Promise<void> {
  await page.getByTestId("last-name-input").fill("Муратова");
  await page.getByTestId("first-name-input").fill("Айдана");
  await page.getByTestId("phone-input").fill("+77001234567");
  await page.getByTestId("iin-input").fill(iin);
  await page.getByTestId("city-select").selectOption("almaty");
  await page.getByTestId("social-checkbox-instagram").check();
  await page.getByTestId("social-input-instagram").fill("aidana_test");
  await page.getByTestId("consent-all").check();
}

// waitForDictionaries blocks the test until the cities select and the
// category container have rendered their API-driven options. Loosened from
// "wait for category-checkbox-beauty" to "wait for any real option / any
// category checkbox" so the test is resilient to seed-data changes — beauty
// is no longer a load-bearing fixture.
async function waitForDictionaries(page: Page): Promise<void> {
  // Cities: count > 1 — the first <option> is the "Выбери город" placeholder,
  // so anything beyond that proves at least one city was rendered.
  await expect
    .poll(async () => page.getByTestId("city-select").locator("option").count(), {
      timeout: 10_000,
    })
    .toBeGreaterThan(1);
  // Categories: at least one category checkbox has been attached. Picks up
  // any category code without baking a specific one into the test.
  await expect
    .poll(async () => page.getByTestId(/^category-checkbox-/).count(), {
      timeout: 10_000,
    })
    .toBeGreaterThan(0);
}

// extractApplicationIdFromBotUrl pulls the application uuid out of the
// success-CTA href. The backend formats it as ?start={uuid}.
function extractApplicationIdFromBotUrl(botUrl: string): string {
  const url = new URL(botUrl);
  const id = url.searchParams.get("start");
  if (!id) throw new Error(`success CTA href has no start param: ${botUrl}`);
  return id;
}

test.describe("Landing submission flow", () => {
  // Surface unexpected JS errors that the form handler catches silently —
  // a missing dependency or a broken event listener would otherwise pass
  // unnoticed and the failure mode (no submit, no error) is hard to debug.
  // console.warn (not log) keeps eslint's no-console rule happy and matches
  // the diagnostic intent — these lines fire only when something goes wrong.
  test.beforeEach(async ({ page }) => {
    page.on("pageerror", (err) => console.warn("[pageerror]", err.message));
    page.on("requestfailed", (req) =>
      console.warn("[requestfailed]", req.url(), req.failure()?.errorText),
    );
  });

  test("Dictionaries — селекты непустые после загрузки", async ({ page }) => {
    // Smoke test that pins the contract between the API and the form: cities
    // dropdown carries real options, categories renders at least one
    // checkbox. Placed first so a regression here fails fast and obviously,
    // before any submit flow runs.
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await waitForDictionaries(page);

    const cityCount = await page.getByTestId("city-select").locator("option").count();
    expect(cityCount).toBeGreaterThan(1);
    const categoryCount = await page.getByTestId(/^category-checkbox-/).count();
    expect(categoryCount).toBeGreaterThan(0);
  });

  test("Golden path — fills the form, sees success screen with telegram CTA", async ({
    page,
  }) => {
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await waitForDictionaries(page);

    await fillRequiredFields(page, uniqueIIN());
    // The <input> itself is `class="hidden"`; clicking the wrapping <label>
    // toggles it and fires the change handler — same UX path a real user takes.
    await page.getByTestId("category-label-beauty").click();

    await page.getByTestId("submit-button").click();

    // Success screen becomes visible (the section flips from "hidden" to "flex").
    await expect(page.getByTestId("success-screen")).toBeVisible();

    // CTA carries a Telegram deep-link with the application id as start param.
    const cta = page.getByTestId("success-cta");
    await expect(cta).toBeVisible();
    const href = await cta.getAttribute("href");
    expect(href).not.toBeNull();
    expect(href ?? "").toMatch(/^https:\/\/t\.me\/[^?]+\?start=[0-9a-f-]{36}$/);

    if (href) created.push(extractApplicationIdFromBotUrl(href));
  });

  test("Other category — categoryOtherText input appears and submit succeeds", async ({
    page,
  }) => {
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await waitForDictionaries(page);

    await fillRequiredFields(page, uniqueIIN());

    // Pick the "other" category — the dedicated text input must surface.
    await page.getByTestId("category-label-other").click();
    const otherInput = page.getByTestId("category-other-input");
    await expect(otherInput).toBeVisible();
    await otherInput.fill("Авторские ASMR-видео про винтажные велосипеды");

    await page.getByTestId("submit-button").click();

    await expect(page.getByTestId("success-screen")).toBeVisible();
    const href = await page.getByTestId("success-cta").getAttribute("href");
    expect(href ?? "").toMatch(/^https:\/\/t\.me\/[^?]+\?start=[0-9a-f-]{36}$/);
    if (href) created.push(extractApplicationIdFromBotUrl(href));
  });

  test("Server validation — under-age IIN surfaces form error, no success", async ({
    page,
  }) => {
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await waitForDictionaries(page);

    await fillRequiredFields(page, underageIIN());
    // The <input> itself is `class="hidden"`; clicking the wrapping <label>
    // toggles it and fires the change handler — same UX path a real user takes.
    await page.getByTestId("category-label-beauty").click();

    await page.getByTestId("submit-button").click();

    // Server returns 422 UNDER_AGE → landing renders the message in form-error
    // and stays on the form (no success transition).
    const formError = page.getByTestId("form-error");
    await expect(formError).toBeVisible();
    await expect(formError).toContainText("Возраст");
    await expect(page.getByTestId("success-screen")).toBeHidden();

    // Submit button must re-enable so the creator can fix the IIN and retry.
    await expect(page.getByTestId("submit-button")).toBeEnabled();
  });
});
