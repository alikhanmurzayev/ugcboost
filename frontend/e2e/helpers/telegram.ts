import type { APIRequestContext } from "@playwright/test";

export interface SentMessageLite {
  chatId: number;
  text: string;
  sentAt: string;
}

export const TG_DEFAULT_POLL_INTERVAL_MS = 250;

// collectTelegramSent polls /test/telegram/sent for `windowMs` and returns
// every record whose chatId matches. The poll is cumulative — a record
// captured at any point during the window stays in the result. Used to
// assert both presence (with subsequent shape checks) and absence.
export async function collectTelegramSent(
  request: APIRequestContext,
  apiUrl: string,
  chatId: number,
  since: string,
  windowMs: number,
  pollIntervalMs: number = TG_DEFAULT_POLL_INTERVAL_MS,
): Promise<SentMessageLite[]> {
  const deadline = Date.now() + windowMs;
  const seen = new Map<string, SentMessageLite>();
  while (Date.now() < deadline) {
    const resp = await request.get(`${apiUrl}/test/telegram/sent`, {
      params: { chatId: String(chatId), since },
    });
    if (resp.status() === 200) {
      const body = (await resp.json()) as {
        data: { messages: SentMessageLite[] };
      };
      for (const m of body.data.messages) {
        seen.set(`${m.chatId}|${m.sentAt}|${m.text}`, m);
      }
    }
    await new Promise((r) => setTimeout(r, pollIntervalMs));
  }
  return Array.from(seen.values());
}
