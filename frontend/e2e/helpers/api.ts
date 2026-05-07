/**
 * Shared E2E helpers for browser tests across web / tma / landing.
 *
 * Each function is a thin HTTP wrapper around the backend test endpoints
 * (POST /test/*) or a deterministic input generator. Algorithms that mirror
 * backend domain logic (IIN checksum, age math) are duplicated here on
 * purpose — frontend e2e is an isolated module and must not import Go code.
 *
 * Request/response types are derived from the OpenAPI schemas regenerated
 * by `make generate-api` (see frontend/e2e/types/{schema,test-schema}.ts).
 * Helper-shaped types (opts/return) extend or pick from those generated
 * types via Partial / Omit so adding a new optional API field does not
 * require touching the helper. Per docs/standards/frontend-types.md, no
 * helper declares a manual interface that duplicates an API shape.
 */
import { randomBytes, randomInt, randomUUID } from "node:crypto";
import type { APIRequestContext } from "@playwright/test";

import type { components } from "../types/schema";
import type { components as testComponents } from "../types/test-schema";

type CreatorApplicationSubmitRequest =
  components["schemas"]["CreatorApplicationSubmitRequest"];
type CreatorApplicationSubmitResult =
  components["schemas"]["CreatorApplicationSubmitResult"];
type CampaignInput = components["schemas"]["CampaignInput"];
type CampaignCreatedResult = components["schemas"]["CampaignCreatedResult"];
type AddCampaignCreatorsInput =
  components["schemas"]["AddCampaignCreatorsInput"];
type SeedUserRequest = testComponents["schemas"]["SeedUserRequest"];
type SeedUserResult = testComponents["schemas"]["SeedUserResult"];
type SeedUserRole = SeedUserRequest["role"];
type CleanupEntityRequest = testComponents["schemas"]["CleanupEntityRequest"];
type SendTelegramMessageRequest =
  testComponents["schemas"]["SendTelegramMessageRequest"];
type SendTelegramMessageResult =
  testComponents["schemas"]["SendTelegramMessageResult"];
type LoginRequest = components["schemas"]["LoginRequest"];
type LoginResult = components["schemas"]["LoginResult"];
type GetCreatorApplicationResult =
  components["schemas"]["GetCreatorApplicationResult"];
type CreatorApplicationDetailData =
  components["schemas"]["CreatorApplicationDetailData"];
type CreatorApprovalResult = components["schemas"]["CreatorApprovalResult"];
type SendPulseInstagramWebhookRequest =
  components["schemas"]["SendPulseInstagramWebhookRequest"];

export type SocialAccountInput = components["schemas"]["SocialAccountInput"];

// MinCreatorAge mirrors domain.MinCreatorAge in the backend. Bumping the
// backend constant means bumping this one too — the underage helper relies
// on it.
export const MIN_CREATOR_AGE = 18;

// generateValidIIN returns a checksum-valid Kazakhstani IIN for the given
// birth date. The serial is drawn from crypto.randomBytes — same parallel-
// safe approach as backend/e2e/testutil/iin.go::UniqueIIN, since
// Math.random() under N>1 workers gives birthday-paradox collisions on the
// partial unique index that guards active applications.
//
// The century byte encodes both sex and the 100-year window: 1/2 → 1800s,
// 3/4 → 1900s, 5/6 → 2000s. We pick by birth year.
export function generateValidIIN(birth: Date): string {
  const year = birth.getUTCFullYear();
  const yy = String(year % 100).padStart(2, "0");
  const mm = String(birth.getUTCMonth() + 1).padStart(2, "0");
  const dd = String(birth.getUTCDate()).padStart(2, "0");
  const century = centuryByteFor(year);

  // Loop until we hit a serial whose checksum is defined (the rare ~1%
  // corner where both passes land on mod=10 forces a re-roll). Bounded so
  // an algorithmic regression cannot spin forever.
  for (let attempt = 0; attempt < 100; attempt++) {
    const serial = String(randomInt(0, 10_000)).padStart(4, "0");
    const prefix = yy + mm + dd + century + serial;
    const check = iinChecksum(prefix);
    if (check !== null) return prefix + String(check);
  }
  throw new Error("generateValidIIN: failed to find a valid checksum after 100 attempts");
}

function centuryByteFor(year: number): string {
  if (year >= 1800 && year < 1900) return "1";
  if (year >= 1900 && year < 2000) return "3";
  if (year >= 2000 && year < 2100) return "5";
  throw new Error(`centuryByteFor: year ${year} outside the supported 1800–2099 range`);
}

// uniqueIIN returns a fresh, checksum-valid IIN comfortably above
// MinCreatorAge. Year (1985..2005), month (1..12), day (1..28 to dodge
// Feb 29) and the 4-digit serial are all drawn from crypto.randomBytes —
// exactly the strategy used by backend testutil.UniqueIIN. With ~70M valid
// IINs in the pool, collisions across parallel workers are vanishingly
// improbable. Day randomisation eliminates the May-15 UTC-midnight age
// race we hit when the birth date was fixed.
export function uniqueIIN(): string {
  const buf = randomBytes(8);
  const year = 1985 + (buf.readUInt8(0) % 21);
  const month = 1 + (buf.readUInt8(1) % 12);
  const day = 1 + (buf.readUInt8(2) % 28);
  const birth = new Date(Date.UTC(year, month - 1, day));
  return generateValidIIN(birth);
}

// underageIIN builds a checksum-valid IIN for a creator who will be a
// couple of years short of MinCreatorAge against the backend's real-time
// clock — clock-independent so the test does not silently start passing
// when real time catches up to a hardcoded year.
export function underageIIN(): string {
  const now = new Date();
  const birth = new Date(
    Date.UTC(now.getUTCFullYear() - (MIN_CREATOR_AGE - 2), now.getUTCMonth(), now.getUTCDate()),
  );
  return generateValidIIN(birth);
}

// postJson centralises Playwright's untyped resp.json() into a single typed
// surface. `as T` here is the cheapest contract — every caller passes the
// matching OpenAPI-derived type, and a runtime validator (zod) was rejected
// as overkill for test infrastructure that already pins assertions against
// drawer text rendered from the same response.
async function postJson<T>(
  request: APIRequestContext,
  url: string,
  data: unknown,
  expectedStatus: number,
): Promise<T> {
  const resp = await request.post(url, { data });
  if (resp.status() !== expectedStatus) {
    throw new Error(`POST ${url}: ${resp.status()} ${await resp.text()}`);
  }
  return (await resp.json()) as T;
}

// cleanupCreatorApplication removes one application via the test cleanup
// endpoint. 204 = deleted, 404 = already gone (idempotent — afterEach
// drains the LIFO stack and the same id is sometimes wiped by a cascade
// from an earlier cleanup). Anything else throws so the failure surfaces
// immediately instead of silently leaking rows into following test runs.
export async function cleanupCreatorApplication(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const body: CleanupEntityRequest = {
    type: "creator_application",
    id: applicationId,
  };
  const resp = await request.post(`${apiUrl}/test/cleanup-entity`, { data: body });
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(
      `cleanupCreatorApplication ${applicationId}: ${resp.status()} ${await resp.text()}`,
    );
  }
}

async function cleanupUser(
  request: APIRequestContext,
  apiUrl: string,
  userId: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const body: CleanupEntityRequest = { type: "user", id: userId };
  const resp = await request.post(`${apiUrl}/test/cleanup-entity`, { data: body });
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(`cleanupUser ${userId}: ${resp.status()} ${await resp.text()}`);
  }
}

export interface SeededUser {
  email: string;
  password: string;
  userId: string;
  cleanup: () => Promise<void>;
}

// seedAdmin creates a fresh admin user with a unique email (Date.now + uuid
// suffix) to keep parallel workers from colliding on the unique-email
// constraint. Password is the canonical "testpass123" used across all e2e
// suites — the e2e password format is intentionally identical so a single
// helper can log into any seeded user.
export function seedAdmin(
  request: APIRequestContext,
  apiUrl: string,
): Promise<SeededUser> {
  return seedUser(request, apiUrl, "admin", "test-admin");
}

// seedBrandManager mirrors seedAdmin for the brand_manager role. Only the
// role and email prefix differ — RoleGuard / sidebar tests need a real
// non-admin to assert the negative case.
export function seedBrandManager(
  request: APIRequestContext,
  apiUrl: string,
): Promise<SeededUser> {
  return seedUser(request, apiUrl, "brand_manager", "test-bm");
}

async function seedUser(
  request: APIRequestContext,
  apiUrl: string,
  role: SeedUserRole,
  prefix: string,
): Promise<SeededUser> {
  const email = `${prefix}-${Date.now()}-${randomUUID().slice(0, 8)}@e2e.test`;
  const password = "testpass123";
  const body: SeedUserRequest = { email, password, role };
  const result = await postJson<SeedUserResult>(
    request,
    `${apiUrl}/test/seed-user`,
    body,
    201,
  );
  const userId = result.data.id;
  return {
    email,
    password,
    userId,
    cleanup: () => cleanupUser(request, apiUrl, userId),
  };
}

// SeedCreatorApplicationOpts derives every field from the OpenAPI submit
// request. Three fields are widened to allow `null` so the spec can pin
// "field absent / explicitly empty" semantics that OpenAPI marks as
// nullable (address, categoryOtherText) plus middleName, where the test
// occasionally drives the helper to omit it from the body entirely.
export type SeedCreatorApplicationOpts = Partial<
  Omit<
    CreatorApplicationSubmitRequest,
    "middleName" | "address" | "categoryOtherText"
  >
> & {
  middleName?: string | null;
  address?: string | null;
  categoryOtherText?: string | null;
};

// SeededCreatorApplication is what tests assert against. We surface the
// exact values posted to the API (so the spec can compare drawer rendering
// 1:1 without re-deriving anything from server state) plus a parsed
// birthDate (decoded from the IIN to spare callers the encoding rules)
// and a per-application cleanup closure that respects E2E_CLEANUP.
export type SeededCreatorApplication = Omit<
  CreatorApplicationSubmitRequest,
  "middleName" | "address" | "categoryOtherText"
> & {
  applicationId: string;
  middleName: string | null;
  address: string | null;
  categoryOtherText: string | null;
  birthDate: Date;
  cleanup: () => Promise<void>;
};

// seedCreatorApplication POSTs to the production /creators/applications
// endpoint (no auth — it's the public landing-form path) so the seeded row
// is indistinguishable from a real submission: triggers the same audit log,
// telegram-link record, consent rows, etc. Defaults form a "minimum valid"
// applicant; opts overrides any field for the scenario under test.
//
// The 10ms pre-POST sleep guarantees deterministic created_at ordering for
// callers that seed several applications back-to-back: Postgres now() is
// microsecond-precision, but on a slow CI runner sequential POSTs can hit
// the same microsecond and fall back to a UUID tiebreak that is non-
// deterministic. 10ms is well above one tick on every supported runner.
export async function seedCreatorApplication(
  request: APIRequestContext,
  apiUrl: string,
  opts: SeedCreatorApplicationOpts = {},
): Promise<SeededCreatorApplication> {
  await sleep(10);

  const uuid = randomUUID();
  const lastName = opts.lastName ?? `e2e-${uuid}-Иванов`;
  const firstName = opts.firstName ?? "Айдана";
  const middleName: string | null =
    opts.middleName !== undefined ? opts.middleName : "Тестовна";
  const iin = opts.iin ?? uniqueIIN();
  const phone = opts.phone ?? "+77001234567";
  const city = opts.city ?? "almaty";
  const address: string | null = opts.address !== undefined ? opts.address : null;
  const categories = opts.categories ?? ["beauty"];
  const categoryOtherText: string | null =
    opts.categoryOtherText !== undefined ? opts.categoryOtherText : null;
  const socials =
    opts.socials ?? [
      { platform: "instagram", handle: `aidana_test_${uuid.slice(0, 8)}` },
    ];
  const acceptedAll = opts.acceptedAll ?? true;

  // Build the request body. middleName / address / categoryOtherText are
  // forwarded to the API only when non-null; null carries "field absent"
  // semantics on our side, which the OpenAPI shape encodes as the field
  // being omitted from the request body.
  const requestBody: CreatorApplicationSubmitRequest = {
    lastName,
    firstName,
    iin,
    phone,
    city,
    categories,
    socials,
    acceptedAll,
  };
  if (middleName !== null) requestBody.middleName = middleName;
  if (address !== null) requestBody.address = address;
  if (categoryOtherText !== null) requestBody.categoryOtherText = categoryOtherText;

  const result = await postJson<CreatorApplicationSubmitResult>(
    request,
    `${apiUrl}/creators/applications`,
    requestBody,
    201,
  );

  return {
    applicationId: result.data.applicationId,
    lastName,
    firstName,
    middleName,
    iin,
    phone,
    city,
    address,
    categories,
    categoryOtherText,
    socials,
    acceptedAll,
    birthDate: parseBirthDateFromIin(iin),
    cleanup: () => cleanupCreatorApplication(request, apiUrl, result.data.applicationId),
  };
}

// parseBirthDateFromIin reverses generateValidIIN: digits 0..5 are yymmdd
// and digit 6 is the century byte (1/2 → 1800s, 3/4 → 1900s, 5/6 → 2000s).
function parseBirthDateFromIin(iin: string): Date {
  const yy = Number(iin.slice(0, 2));
  const mm = Number(iin.slice(2, 4));
  const dd = Number(iin.slice(4, 6));
  const century = iin.charAt(6);
  let yearBase: number;
  if (century === "1" || century === "2") yearBase = 1800;
  else if (century === "3" || century === "4") yearBase = 1900;
  else if (century === "5" || century === "6") yearBase = 2000;
  else throw new Error(`parseBirthDateFromIin: unknown century byte ${century}`);
  return new Date(Date.UTC(yearBase + yy, mm - 1, dd));
}

// uniqueTelegramUserId picks ids well above the realistic Telegram range
// (positive int64 above 2^30) so synthetic test users can never collide
// with a real user during a manual smoke through the live bot. The lower
// bits come from crypto.randomBytes — same parallel-worker safety we get
// from crypto/rand in backend testutil.UniqueTelegramUserID, without the
// per-process atomic counter that resets on every fresh Node process.
//
// 6 random bytes ≈ 2^48 of entropy; epoch + max stays inside
// Number.MAX_SAFE_INTEGER (2^53 - 1).
export function uniqueTelegramUserId(): number {
  const epoch = 1 << 30;
  const buf = randomBytes(6);
  let rand = 0;
  for (let i = 0; i < buf.length; i++) {
    rand = rand * 256 + buf.readUInt8(i);
  }
  return epoch + rand;
}

// LinkTelegramOpts lets the spec pin synthetic identity fields when a test
// asserts on telegram username/first/last name in the drawer header. By
// default the helper picks fresh values per call. Field names mirror the
// SendTelegramMessageRequest schema with telegramUserId substituted for
// chatId/userId (the helper sets both from one source of truth).
export interface LinkTelegramOpts {
  telegramUserId?: number;
  username?: string;
  firstName?: string;
  lastName?: string;
}

export interface LinkedTelegram {
  telegramUserId: number;
  username: string;
  firstName: string;
  lastName: string;
}

// linkTelegramToApplication injects a synthetic "/start <applicationId>"
// update into the in-process Telegram handler via POST /test/telegram/message.
// The handler runs synchronously, so once this resolves the application is
// linked — UI can be navigated immediately without waitFor.
export async function linkTelegramToApplication(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
  opts: LinkTelegramOpts = {},
): Promise<LinkedTelegram> {
  const telegramUserId = opts.telegramUserId ?? uniqueTelegramUserId();
  const username = opts.username ?? `tg_${telegramUserId}`;
  const firstName = opts.firstName ?? "Тест";
  const lastName = opts.lastName ?? "Креатор";
  const body: SendTelegramMessageRequest = {
    chatId: telegramUserId,
    userId: telegramUserId,
    text: `/start ${applicationId}`,
    username,
    firstName,
    lastName,
  };
  await postJson<SendTelegramMessageResult>(
    request,
    `${apiUrl}/test/telegram/message`,
    body,
    200,
  );
  return { telegramUserId, username, firstName, lastName };
}

// iinChecksum runs the two-pass Republic of Kazakhstan algorithm on the
// first 11 digits of an IIN. Returns the checksum digit (0..9) or null if
// both passes land on the forbidden mod=10 result.
function iinChecksum(first11: string): number | null {
  const digits = first11.split("").map((ch) => Number(ch));
  const w1 = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11];
  const w2 = [3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 2];
  const weighted = (weights: number[]): number =>
    weights.reduce((acc, w, i) => {
      const d = digits[i];
      if (d === undefined) throw new Error("iinChecksum: digit index out of bounds");
      return acc + d * w;
    }, 0);

  let mod = weighted(w1) % 11;
  if (mod === 10) {
    mod = weighted(w2) % 11;
    if (mod === 10) return null;
  }
  return mod;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// loginAsAdmin authenticates the seeded admin via POST /auth/login at the
// API level (not through the UI form) and returns the access token suitable
// for an Authorization: Bearer header. Used by tests that need to call
// admin endpoints directly — e.g. read application detail to discover a
// social uuid before driving the UI.
export async function loginAsAdmin(
  request: APIRequestContext,
  apiUrl: string,
  email: string,
  password: string,
): Promise<string> {
  const body: LoginRequest = { email, password };
  const result = await postJson<LoginResult>(
    request,
    `${apiUrl}/auth/login`,
    body,
    200,
  );
  return result.data.accessToken;
}

// fetchApplicationDetail GETs /creators/applications/{id} with the admin
// bearer and returns the unwrapped detail. Tests use it to look up the
// stable uuid of a specific social before clicking a verify button keyed by
// that id, and to assert post-action server state in the same flow.
export async function fetchApplicationDetail(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
  token: string,
): Promise<CreatorApplicationDetailData> {
  const resp = await request.get(
    `${apiUrl}/creators/applications/${applicationId}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (resp.status() !== 200) {
    throw new Error(
      `fetchApplicationDetail ${applicationId}: ${resp.status()} ${await resp.text()}`,
    );
  }
  const result = (await resp.json()) as GetCreatorApplicationResult;
  return result.data;
}

// manualVerifyApplicationSocial drives the admin manual-verify endpoint
// (POST /creators/applications/{id}/socials/{socialId}/verify). Status-only
// check; response body is not validated (the endpoint returns EmptyResult
// per OpenAPI). Used to walk a TT-only application from `verification` to
// `moderation` without going through the SendPulse webhook. Caller is
// responsible for asserting that the application is in `verification`
// before invoking — the endpoint returns 422 on `moderation`.
export async function manualVerifyApplicationSocial(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
  socialId: string,
  token: string,
): Promise<void> {
  const resp = await request.post(
    `${apiUrl}/creators/applications/${applicationId}/socials/${socialId}/verify`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: {},
    },
  );
  if (resp.status() !== 200) {
    throw new Error(
      `manualVerifyApplicationSocial ${applicationId}/${socialId}: ${resp.status()} ${await resp.text()}`,
    );
  }
}

async function cleanupCreator(
  request: APIRequestContext,
  apiUrl: string,
  creatorId: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const body: CleanupEntityRequest = { type: "creator", id: creatorId };
  const resp = await request.post(`${apiUrl}/test/cleanup-entity`, { data: body });
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(`cleanupCreator ${creatorId}: ${resp.status()} ${await resp.text()}`);
  }
}

// approveApplication drives admin POST /creators/applications/{id}/approve and
// returns the freshly-materialised creator id. Caller is responsible for
// having walked the application up to `moderation` first (link Telegram +
// verify all socials).
export async function approveApplication(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
  token: string,
): Promise<string> {
  const resp = await request.post(
    `${apiUrl}/creators/applications/${applicationId}/approve`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (resp.status() !== 200) {
    throw new Error(
      `approveApplication ${applicationId}: ${resp.status()} ${await resp.text()}`,
    );
  }
  const result = (await resp.json()) as CreatorApprovalResult;
  return result.data.creatorId;
}

export type SeedApprovedCreatorOpts = SeedCreatorApplicationOpts;

export interface SeededApprovedCreator {
  creatorId: string;
  applicationId: string;
  application: SeededCreatorApplication;
  telegram: LinkedTelegram;
  cleanup: () => Promise<void>;
}

// seedApprovedCreator composes the full happy path that materialises a row in
// `creators`: seed an application via the public endpoint, link Telegram so
// moderation is unblocked, manually verify every social (admin path — keeps
// the helper sync without polling SendPulse), then approve as admin. Cleanup
// drops the creator row first (cascade-safe) and then the originating
// application — matching defer-LIFO ordering callers expect.
export async function seedApprovedCreator(
  request: APIRequestContext,
  apiUrl: string,
  adminToken: string,
  opts: SeedApprovedCreatorOpts = {},
): Promise<SeededApprovedCreator> {
  const application = await seedCreatorApplication(request, apiUrl, opts);
  try {
    const telegram = await linkTelegramToApplication(
      request,
      apiUrl,
      application.applicationId,
    );
    const detail = await fetchApplicationDetail(
      request,
      apiUrl,
      application.applicationId,
      adminToken,
    );
    for (const social of detail.socials) {
      if (!social.verified) {
        await manualVerifyApplicationSocial(
          request,
          apiUrl,
          application.applicationId,
          social.id,
          adminToken,
        );
      }
    }
    const creatorId = await approveApplication(
      request,
      apiUrl,
      application.applicationId,
      adminToken,
    );

    return {
      creatorId,
      applicationId: application.applicationId,
      application,
      telegram,
      cleanup: async () => {
        try {
          await cleanupCreator(request, apiUrl, creatorId);
        } finally {
          await application.cleanup();
        }
      },
    };
  } catch (err) {
    // Partial failure: roll back the application we already created so the row
    // does not leak into the next test run. Cleanup errors are swallowed —
    // surfacing the original failure is more useful than a chained 5xx.
    await application.cleanup().catch(() => {});
    throw err;
  }
}

export async function cleanupCampaign(
  request: APIRequestContext,
  apiUrl: string,
  campaignId: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const body: CleanupEntityRequest = { type: "campaign", id: campaignId };
  const resp = await request.post(`${apiUrl}/test/cleanup-entity`, { data: body });
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(`cleanupCampaign ${campaignId}: ${resp.status()} ${await resp.text()}`);
  }
}

export type SeedCampaignOpts = Partial<CampaignInput>;

export interface SeededCampaign {
  campaignId: string;
  name: string;
  tmaUrl: string;
  cleanup: () => Promise<void>;
}

// addCampaignCreators drives admin POST /campaigns/{id}/creators (A1) which
// the campaign-creators-frontend slice 1/2 spec relies on to seed rows the
// read-only section then renders. Add-via-UI ships in slice 2/2; until then
// the smoke test stands in by hitting the API directly with the admin
// bearer. The campaigns FK on campaign_creators is not ON DELETE CASCADE,
// so callers must detach the rows (via `removeCampaignCreator`) before
// `cleanupCampaign` runs.
export async function addCampaignCreators(
  request: APIRequestContext,
  apiUrl: string,
  campaignId: string,
  adminToken: string,
  creatorIds: string[],
): Promise<void> {
  const body: AddCampaignCreatorsInput = { creatorIds };
  const resp = await request.post(`${apiUrl}/campaigns/${campaignId}/creators`, {
    headers: { Authorization: `Bearer ${adminToken}` },
    data: body,
  });
  if (resp.status() !== 201) {
    throw new Error(
      `addCampaignCreators ${campaignId}: ${resp.status()} ${await resp.text()}`,
    );
  }
}

// removeCampaignCreator drives admin DELETE /campaigns/{id}/creators/{creatorId}
// (A2) — used as a cleanup step that releases the campaign_creators FK
// before the campaign / creator cleanup helpers fire. 204 = removed,
// 404 = already gone (idempotent against cascading orders).
export async function removeCampaignCreator(
  request: APIRequestContext,
  apiUrl: string,
  campaignId: string,
  creatorId: string,
  adminToken: string,
): Promise<void> {
  if (process.env.E2E_CLEANUP === "false") return;
  const resp = await request.delete(
    `${apiUrl}/campaigns/${campaignId}/creators/${creatorId}`,
    { headers: { Authorization: `Bearer ${adminToken}` } },
  );
  if (resp.status() !== 204 && resp.status() !== 404) {
    throw new Error(
      `removeCampaignCreator ${campaignId}/${creatorId}: ${resp.status()} ${await resp.text()}`,
    );
  }
}

// seedCampaign drives the production POST /campaigns endpoint with an admin
// bearer so the seeded row is indistinguishable from a real admin-created
// campaign (same audit log, same uniqueness guarantees). Defaults pick a
// uuid-suffixed name to keep parallel workers from colliding on the unique
// `name` index. Cleanup uses /test/cleanup-entity type=campaign and is
// idempotent (404 = already gone).
export async function seedCampaign(
  request: APIRequestContext,
  apiUrl: string,
  adminToken: string,
  opts: SeedCampaignOpts = {},
): Promise<SeededCampaign> {
  const uuid = randomUUID();
  const name = opts.name ?? `e2e-campaign-${uuid.slice(0, 8)}`;
  const tmaUrl =
    opts.tmaUrl ?? `https://t.me/ugcboost_bot/app?startapp=${uuid.slice(0, 8)}`;
  const body: CampaignInput = { name, tmaUrl };
  const resp = await request.post(`${apiUrl}/campaigns`, {
    headers: { Authorization: `Bearer ${adminToken}` },
    data: body,
  });
  if (resp.status() !== 201) {
    throw new Error(`seedCampaign: ${resp.status()} ${await resp.text()}`);
  }
  const result = (await resp.json()) as CampaignCreatedResult;
  const campaignId = result.data.id;
  return {
    campaignId,
    name,
    tmaUrl,
    cleanup: () => cleanupCampaign(request, apiUrl, campaignId),
  };
}

// SendPulseWebhookSecret defaults to the local-dev value baked into
// backend/.env so out-of-the-box `make test-e2e-frontend` works against a
// hand-spun stack. Override via the SENDPULSE_WEBHOOK_SECRET env var when
// the backend is started with a non-default secret.
const DEFAULT_SENDPULSE_SECRET = "local-dev-sendpulse-secret";

function sendPulseSecret(): string {
  return process.env.SENDPULSE_WEBHOOK_SECRET || DEFAULT_SENDPULSE_SECRET;
}

// triggerSendPulseInstagramWebhook posts the canonical "verified" payload
// for the given application — mimicking what SendPulse sends when the
// creator DMs their UGC-NNNNNN code to the Instagram bot. After the call
// returns 200 the application's IG social is auto-verified and the
// application sits in `verification` (single-social) or already in
// `moderation` (multi-social with that one auto-verified row), per backend
// state machine. Caller is expected to assert post-state via
// fetchApplicationDetail.
export async function triggerSendPulseInstagramWebhook(
  request: APIRequestContext,
  apiUrl: string,
  opts: { username: string; verificationCode: string },
): Promise<void> {
  const body: SendPulseInstagramWebhookRequest = {
    username: opts.username,
    lastMessage: `Hi UGCBoost! My code is ${opts.verificationCode}`,
    contactId: `contact-${opts.verificationCode}`,
  };
  const resp = await request.post(`${apiUrl}/webhooks/sendpulse/instagram`, {
    headers: { Authorization: `Bearer ${sendPulseSecret()}` },
    data: body,
  });
  if (resp.status() !== 200) {
    throw new Error(
      `triggerSendPulseInstagramWebhook: ${resp.status()} ${await resp.text()}`,
    );
  }
}
