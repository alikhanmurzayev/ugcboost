// Captures the five canonical UTM markers from the landing-page URL into
// sessionStorage so a creator who lands on a tagged link and then submits the
// application carries the source attribution all the way through to the
// backend. Last-click model: a fresh URL with any UTM key overwrites the
// stored payload; a reload without UTM keeps the previous capture so the
// landing form does not lose the marker on F5.

export type UtmKey =
  | "utm_source"
  | "utm_medium"
  | "utm_campaign"
  | "utm_term"
  | "utm_content";

export type UtmPayload = Partial<Record<UtmKey, string>>;

export const UTM_KEYS: readonly UtmKey[] = [
  "utm_source",
  "utm_medium",
  "utm_campaign",
  "utm_term",
  "utm_content",
];

const STORAGE_KEY = "ugc_utm";

export function captureUTM(search?: string): void {
  if (typeof window === "undefined") return;
  const query = search ?? window.location.search;
  const params = new URLSearchParams(query);
  const captured: UtmPayload = {};
  for (const key of UTM_KEYS) {
    const value = params.get(key);
    if (value === null) continue;
    const trimmed = value.trim();
    if (trimmed === "") continue;
    captured[key] = trimmed;
  }
  if (Object.keys(captured).length === 0) return;
  try {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(captured));
  } catch {
    // sessionStorage unavailable (Safari private mode, sandboxed iframe);
    // capture failure is non-blocking — UTM tracking is best-effort metadata.
  }
}

export function readUTM(): UtmPayload {
  if (typeof window === "undefined") return {};
  let raw: string | null;
  try {
    raw = window.sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return {};
  }
  if (raw === null || raw === "") return {};

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {};
  }
  if (!isStringRecord(parsed)) return {};

  const out: UtmPayload = {};
  for (const key of UTM_KEYS) {
    const value = readStringField(parsed, key);
    if (value !== undefined) out[key] = value;
  }
  return out;
}

function isStringRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readStringField(record: Record<string, unknown>, key: UtmKey): string | undefined {
  const raw = record[key];
  if (typeof raw !== "string") return undefined;
  const trimmed = raw.trim();
  return trimmed === "" ? undefined : trimmed;
}
