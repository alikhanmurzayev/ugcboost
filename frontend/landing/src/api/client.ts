import createClient from "openapi-fetch";
import type { paths } from "./generated/schema";

declare global {
  interface Window {
    __RUNTIME_CONFIG__?: { apiUrl?: string };
  }
}

function getApiBase(): string {
  if (typeof window !== "undefined" && window.__RUNTIME_CONFIG__?.apiUrl) {
    return window.__RUNTIME_CONFIG__.apiUrl;
  }
  return "/api";
}

const BASE = getApiBase();
export { BASE as apiBase };

export class ApiError extends Error {
  status: number;
  code: string;
  // serverMessage preserves the human-readable message returned by the API so
  // the landing form can show it verbatim ("Невалидный ИИН", "Возраст менее
  // 21 лет" и т.п.). Keep it separate from the JS Error.message contract:
  // callers can either fall back to err.message (which we still set to the
  // server message via super()) or branch on err.code for code-driven UI.
  serverMessage: string;

  constructor(status: number, code: string, serverMessage?: string) {
    super(serverMessage ?? code);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.serverMessage = serverMessage ?? "";
  }
}

// Landing reaches only public endpoints (dictionaries + creator submit), so
// the client carries no auth middleware and no credentials. Compare with
// frontend/web/src/api/client.ts: web layers onRequest/onResponse plus a
// rawClient for refresh; that machinery is intentionally absent here.
const client = createClient<paths>({ baseUrl: BASE });

export default client;
