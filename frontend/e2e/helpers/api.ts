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
