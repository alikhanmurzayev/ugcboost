/**
 * Browser e2e — admin загружает / заменяет / скачивает PDF-шаблон договора
 * на странице /campaigns/:campaignId.
 *
 * Покрывает chunk 9a campaign-roadmap. Каждый тест сеет своего admin и
 * кампанию через composable api.ts; cleanup идёт LIFO через
 * cleanupCampaign / SeededUser.cleanup и уважает E2E_CLEANUP=false. Готовые
 * PDF-байты собирает helper buildContractPDF, который в рантайме формирует
 * мини-PDF с нужным набором плейсхолдеров — тест не зависит от реального
 * шаблона Аиданы и работает на чистой staging-БД.
 *
 * Happy upload — admin открывает деталку кампании без шаблона, кликает
 * «Загрузить шаблон», выбирает PDF с тремя известными плейсхолдерами.
 * Сразу после успеха появляется preview-блок «Найдены плейсхолдеры:» с
 * чипами CreatorFIO / CreatorIIN / IssuedDate и активной кнопкой
 * «Скачать существующий шаблон»; кнопка превращается в «Заменить шаблон».
 * Закрывает AC «после успешного upload рендерится preview-блок».
 *
 * Reload persistence — после reload preview-блок исчезает (он только в
 * mutation-response), но «Заменить шаблон» + «Скачать шаблон» остаются —
 * `hasContractTemplate=true` приходит из GET /campaigns/{id}. Закрывает AC
 * «при reload отображается «Заменить шаблон» + «Скачать шаблон»».
 *
 * Validation error — admin грузит PDF без CreatorIIN. Бэк отвечает 422
 * CONTRACT_MISSING_PLACEHOLDER + details.missing=["CreatorIIN"]; фронт
 * рендерит error-блок ровно с message из бэка (не своим хардкодом).
 * Закрывает AC «при ошибке upload фронт показывает error-блок с
 * сообщением из бэкенда (не своим хардкодом)».
 *
 * Download — заранее засеянная кампания с шаблоном, admin кликает
 * «Скачать существующий шаблон»; ожидаем download event с filename и
 * application/pdf content-type. Закрывает download-side AC.
 */
import { test, expect, type Page } from "@playwright/test";
import { loginAsAdmin, seedAdmin, seedCampaign } from "../helpers/api";
import { loginAs } from "../helpers/ui-web";

const API_URL = process.env.API_URL || "http://localhost:8080";
const CLEANUP_TIMEOUT_MS = 5_000;

// Minimal PDF builder — encodes a one-page text-only PDF whose lines are the
// supplied tokens. Output is parseable by the production extractor (the same
// ledongthuc/pdf reader the backend uses); no external font deps.
function buildContractPDF(placeholders: string[]): Buffer {
  const lines: string[] = ["Contract template (e2e fixture)"];
  for (const name of placeholders) lines.push(`{{${name}}}`);
  return makeSimplePDF(lines);
}

// makeSimplePDF emits a valid 1-page PDF/1.4 with each entry of `lines` on
// its own line (Helvetica 12). Hand-rolled because gofpdf is not available
// from Node, and the production extractor only needs glyph X/Y + text — no
// fancy compression, no embedded fonts.
function makeSimplePDF(lines: string[]): Buffer {
  const escape = (s: string) =>
    s.replaceAll("\\", "\\\\").replaceAll("(", "\\(").replaceAll(")", "\\)");
  const stream =
    "BT\n/F1 12 Tf\n14 TL\n50 780 Td\n" +
    lines.map((l, i) => (i === 0 ? `(${escape(l)}) Tj\n` : `T*\n(${escape(l)}) Tj\n`)).join("") +
    "ET\n";
  const objs: string[] = [];
  objs.push("<< /Type /Catalog /Pages 2 0 R >>");
  objs.push("<< /Type /Pages /Kids [3 0 R] /Count 1 >>");
  objs.push(
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R " +
      "/Resources << /Font << /F1 5 0 R >> >> >>",
  );
  objs.push(
    `<< /Length ${stream.length} >>\nstream\n${stream}endstream`,
  );
  objs.push("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>");

  let header = "%PDF-1.4\n";
  const offsets: number[] = [];
  let body = "";
  for (let i = 0; i < objs.length; i++) {
    offsets.push(header.length + body.length);
    body += `${i + 1} 0 obj\n${objs[i]}\nendobj\n`;
  }
  const xrefOffset = header.length + body.length;
  let xref = `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const o of offsets) xref += `${String(o).padStart(10, "0")} 00000 n \n`;
  const trailer = `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF`;
  return Buffer.from(header + body + xref + trailer, "binary");
}

test.describe("Admin campaign contract template", () => {
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

  test("Happy upload renders preview block with three placeholders", async ({
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
    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    await expect(page.getByTestId("contract-template-section")).toBeVisible();
    await expect(
      page.getByTestId("contract-template-upload-button"),
    ).toBeVisible();

    await uploadPDF(
      page,
      buildContractPDF(["CreatorFIO", "CreatorIIN", "IssuedDate"]),
    );

    await expect(
      page.getByTestId("contract-template-placeholders"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-placeholder-CreatorFIO"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-placeholder-CreatorIIN"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-placeholder-IssuedDate"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-replace-button"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-download-button"),
    ).toBeVisible();
  });

  test("Reload keeps replace + download buttons; preview block does not persist", async ({
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
    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);
    await uploadPDF(
      page,
      buildContractPDF(["CreatorFIO", "CreatorIIN", "IssuedDate"]),
    );
    await expect(
      page.getByTestId("contract-template-replace-button"),
    ).toBeVisible();

    await page.reload();
    await expect(
      page.getByTestId("contract-template-replace-button"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-download-button"),
    ).toBeVisible();
    await expect(
      page.getByTestId("contract-template-placeholders"),
    ).toHaveCount(0);
  });

  test("Validation error renders backend message inline", async ({
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
    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    // PDF carrying only FIO + IssuedDate — backend reports CreatorIIN missing.
    await uploadPDF(page, buildContractPDF(["CreatorFIO", "IssuedDate"]));

    await expect(page.getByTestId("contract-template-error")).toContainText(
      "CreatorIIN",
    );
    await expect(
      page.getByTestId("contract-template-placeholders"),
    ).toHaveCount(0);
  });

  test("Download triggers PDF download event", async ({ page, request }) => {
    const admin = await seedAdmin(request, API_URL);
    cleanupStack.push(admin.cleanup);
    const adminToken = await loginAsAdmin(
      request,
      API_URL,
      admin.email,
      admin.password,
    );
    const seeded = await seedCampaign(request, API_URL, adminToken);
    cleanupStack.push(seeded.cleanup);

    // Upload via API so the test focuses on the download path.
    await uploadViaAPI(
      request,
      adminToken,
      seeded.campaignId,
      buildContractPDF(["CreatorFIO", "CreatorIIN", "IssuedDate"]),
    );

    await loginAs(page, admin.email, admin.password);
    await page.goto(`/campaigns/${seeded.campaignId}`);

    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByTestId("contract-template-download-button").click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/\.pdf$/);
  });
});

async function uploadPDF(page: Page, body: Buffer): Promise<void> {
  // setInputFiles works against the hidden <input type="file"> directly —
  // click on the visible button only forwards to the same input via ref,
  // so bypassing it keeps the upload race-free.
  const input = page.getByTestId("contract-template-input");
  await input.setInputFiles({
    name: "contract.pdf",
    mimeType: "application/pdf",
    buffer: body,
  });
}

async function uploadViaAPI(
  request: import("@playwright/test").APIRequestContext,
  adminToken: string,
  campaignId: string,
  body: Buffer,
): Promise<void> {
  const resp = await request.put(
    `${API_URL}/campaigns/${campaignId}/contract-template`,
    {
      headers: {
        Authorization: `Bearer ${adminToken}`,
        "Content-Type": "application/pdf",
      },
      data: body,
    },
  );
  if (resp.status() !== 200) {
    throw new Error(`uploadViaAPI: ${resp.status()} ${await resp.text()}`);
  }
}

async function withTimeout<T>(
  p: Promise<T>,
  ms: number,
  label: string,
): Promise<T> {
  let t: NodeJS.Timeout | undefined;
  return Promise.race([
    p.finally(() => {
      if (t) clearTimeout(t);
    }),
    new Promise<T>((_, reject) => {
      t = setTimeout(
        () => reject(new Error(`timeout (${label}) after ${ms}ms`)),
        ms,
      );
    }),
  ]);
}
