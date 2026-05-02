/**
 * Shared E2E helpers for browser tests across web / tma / landing.
 *
 * Each function is a thin HTTP wrapper around the backend test endpoints
 * (POST /test/*) or a deterministic input generator
 * (generateValidIIN / uniqueIIN / underageIIN). Algorithms that mirror
 * backend domain logic (IIN checksum, age math) are duplicated here on
 * purpose — frontend e2e is an isolated module and must not import Go code.
 */
import type { APIRequestContext } from "@playwright/test";

// MinCreatorAge mirrors domain.MinCreatorAge in the backend. Bumping the
// backend constant means bumping this one too — the underage helper relies
// on it.
export const MIN_CREATOR_AGE = 18;

// generateValidIIN returns a checksum-valid Kazakhstani IIN for the given
// birth date. The serial is randomised, so two calls on the same date give
// different IINs unless extremely (un)lucky (1/10000 collision per call).
// The century byte encodes both sex and the 100-year window: 1/2 → 1800s,
// 3/4 → 1900s, 5/6 → 2000s. We pick by birth year.
export function generateValidIIN(birth: Date): string {
  const year = birth.getUTCFullYear();
  const yy = String(year % 100).padStart(2, "0");
  const mm = String(birth.getUTCMonth() + 1).padStart(2, "0");
  const dd = String(birth.getUTCDate()).padStart(2, "0");
  const century = centuryByteFor(year);

  // Loop until we hit a serial that yields a valid checksum (rare miss when
  // both passes land on the forbidden mod=10 result — happens for ~1% of
  // serials, bounded retry keeps the function deterministic in practice).
  for (let attempt = 0; attempt < 100; attempt++) {
    const serial = String(Math.floor(Math.random() * 10_000)).padStart(4, "0");
    const prefix = yy + mm + dd + century + serial;
    const check = iinChecksum(prefix);
    if (check !== null) return prefix + String(check);
  }
  throw new Error("generateValidIIN: failed to find a valid checksum after 100 attempts");
}

// centuryByteFor mirrors backend domain.iinYear: 1/2 → 1800s, 3/4 → 1900s,
// 5/6 → 2000s. We return the male digit (1/3/5) — the backend treats 1+2,
// 3+4, 5+6 as the same year mapping, so picking either works.
function centuryByteFor(year: number): string {
  if (year >= 1800 && year < 1900) return "1";
  if (year >= 1900 && year < 2000) return "3";
  if (year >= 2000 && year < 2100) return "5";
  throw new Error(`centuryByteFor: year ${year} outside the supported 1800–2099 range`);
}

// uniqueIIN builds an IIN for an applicant ~comfortably above MinCreatorAge.
// Uses Date.now to seed birth year so different test runs land on slightly
// different birthdays — combined with the random serial this avoids partial
// unique index collisions across parallel workers without any shared state.
export function uniqueIIN(): string {
  const birth = new Date(Date.UTC(1995, 4, 15)); // 1995-05-15, ~30 years old in 2026
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

// cleanupCreatorApplication removes one application via the test cleanup
// endpoint. Cascades drop the related categories / socials / consents rows
// (DELETE CASCADE in migrations); audit_logs survive on purpose.
export async function cleanupCreatorApplication(
  request: APIRequestContext,
  apiUrl: string,
  applicationId: string,
): Promise<void> {
  await request.post(`${apiUrl}/test/cleanup-entity`, {
    data: { type: "creator_application", id: applicationId },
  });
}

// SeededUser is what seedAdmin / seedBrandManager hand back to the test:
// credentials usable for the UI login plus a per-user cleanup closure that
// honours E2E_CLEANUP=false (data preserved for post-mortem on failure).
export interface SeededUser {
  email: string;
  password: string;
  userId: string;
  cleanup: () => Promise<void>;
}

interface SeedUserApiResponse {
  data: { id: string; email: string; role: string };
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
  role: "admin" | "brand_manager",
  prefix: string,
): Promise<SeededUser> {
  const email = `${prefix}-${Date.now()}-${crypto.randomUUID().slice(0, 8)}@e2e.test`;
  const password = "testpass123";
  const resp = await request.post(`${apiUrl}/test/seed-user`, {
    data: { email, password, role },
  });
  if (resp.status() !== 201) {
    throw new Error(`seedUser ${role}: ${resp.status()} ${await resp.text()}`);
  }
  const body = (await resp.json()) as SeedUserApiResponse;
  const userId = body.data.id;
  return {
    email,
    password,
    userId,
    cleanup: async () => {
      if (process.env.E2E_CLEANUP === "false") return;
      await request.post(`${apiUrl}/test/cleanup-entity`, {
        data: { type: "user", id: userId },
      });
    },
  };
}

// SocialAccountInput mirrors backend SocialAccountInput in the OpenAPI spec
// for POST /creators/applications. We duplicate the type instead of importing
// from web's generated schema because frontend e2e is an isolated module
// (per docs/standards/frontend-testing-e2e.md).
export interface SocialAccountInput {
  platform: "instagram" | "tiktok" | "threads";
  handle: string;
}

// SeedCreatorApplicationOpts lets the test override any default field; passing
// `null` for an optional string deletes the default (e.g. middleName: null
// produces a two-word full name in the drawer header).
export interface SeedCreatorApplicationOpts {
  lastName?: string;
  firstName?: string;
  middleName?: string | null;
  iin?: string;
  phone?: string;
  city?: string;
  address?: string | null;
  categories?: string[];
  categoryOtherText?: string | null;
  socials?: SocialAccountInput[];
  acceptedAll?: boolean;
}

// SeededCreatorApplication is what tests assert against. We surface the exact
// values posted to the API so the spec can compare drawer rendering 1:1
// without re-deriving anything from server state. birthDate is parsed back
// from the IIN so callers do not need to know the IIN-encoding rules.
export interface SeededCreatorApplication {
  applicationId: string;
  lastName: string;
  firstName: string;
  middleName: string | null;
  iin: string;
  phone: string;
  city: string;
  address: string | null;
  categories: string[];
  categoryOtherText: string | null;
  socials: SocialAccountInput[];
  birthDate: Date;
  cleanup: () => Promise<void>;
}

interface SubmitApiResponse {
  data: { applicationId: string; telegramBotUrl: string };
}

// seedCreatorApplication POSTs to the production /creators/applications
// endpoint (no auth — it's the public landing-form path) so the seeded row
// is indistinguishable from a real submission: triggers the same audit log,
// telegram-link record, consent rows, etc. Defaults form a "minimum valid"
// applicant; opts overrides any field for the scenario under test.
export async function seedCreatorApplication(
  request: APIRequestContext,
  apiUrl: string,
  opts: SeedCreatorApplicationOpts = {},
): Promise<SeededCreatorApplication> {
  const uuid = crypto.randomUUID();
  const lastName = opts.lastName ?? `e2e-${uuid}-Иванов`;
  const firstName = opts.firstName ?? "Айдана";
  const middleName =
    opts.middleName === undefined ? "Тестовна" : opts.middleName;
  const iin = opts.iin ?? uniqueIIN();
  const phone = opts.phone ?? "+77001234567";
  const city = opts.city ?? "almaty";
  const address = opts.address === undefined ? null : opts.address;
  const categories = opts.categories ?? ["beauty"];
  const categoryOtherText =
    opts.categoryOtherText === undefined ? null : opts.categoryOtherText;
  const socials =
    opts.socials ?? [
      { platform: "instagram", handle: `aidana_test_${uuid.slice(0, 8)}` },
    ];
  const acceptedAll = opts.acceptedAll ?? true;

  const requestBody: Record<string, unknown> = {
    lastName,
    firstName,
    iin,
    phone,
    city,
    categories,
    socials,
    acceptedAll,
  };
  if (middleName) requestBody.middleName = middleName;
  if (address !== null) requestBody.address = address;
  if (categoryOtherText !== null) requestBody.categoryOtherText = categoryOtherText;

  const resp = await request.post(`${apiUrl}/creators/applications`, {
    data: requestBody,
  });
  if (resp.status() !== 201) {
    throw new Error(
      `seedCreatorApplication: ${resp.status()} ${await resp.text()}`,
    );
  }
  const body = (await resp.json()) as SubmitApiResponse;
  const applicationId = body.data.applicationId;

  return {
    applicationId,
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
    birthDate: parseBirthDateFromIin(iin),
    cleanup: () => cleanupCreatorApplication(request, apiUrl, applicationId),
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

// uniqueTelegramUserId mirrors testutil.UniqueTelegramUserID in the backend:
// epoch (1<<30) + (Date.now() % (1<<20)) * 1024 + per-process counter. Picks
// ids well outside the realistic Telegram range so synthetic test users can
// never collide with a real user during a manual smoke against the live bot.
let telegramTestIdCounter = 0;
export function uniqueTelegramUserId(): number {
  const epoch = 1 << 30;
  telegramTestIdCounter += 1;
  return epoch + (Date.now() % (1 << 20)) * 1024 + telegramTestIdCounter;
}

// LinkTelegramOpts lets the spec pin synthetic identity fields when a test
// asserts on telegram username/first/last name in the drawer header. By
// default the helper picks fresh values per call.
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
  const resp = await request.post(`${apiUrl}/test/telegram/message`, {
    data: {
      chatId: telegramUserId,
      userId: telegramUserId,
      text: `/start ${applicationId}`,
      username,
      firstName,
      lastName,
    },
  });
  if (!resp.ok()) {
    throw new Error(
      `linkTelegramToApplication: ${resp.status()} ${await resp.text()}`,
    );
  }
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
