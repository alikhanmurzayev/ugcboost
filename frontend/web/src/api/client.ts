import createClient from "openapi-fetch";
import type { paths } from "./generated/schema";
import { useAuthStore } from "@/stores/auth";

function getApiBase(): string {
  if (typeof window !== "undefined" && window.__RUNTIME_CONFIG__?.apiUrl) {
    return window.__RUNTIME_CONFIG__.apiUrl;
  }
  return "/api"; // Vite dev proxy fallback
}

const BASE = getApiBase();
export { BASE as apiBase };

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string) {
    super(code);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

let refreshPromise: Promise<void> | null = null;

async function refreshToken(): Promise<void> {
  const res = await fetch(`${BASE}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });

  if (!res.ok) {
    useAuthStore.getState().clearAuth();
    throw new ApiError(401, "UNAUTHORIZED");
  }

  const body = await res.json();
  const { accessToken, user } = body.data;
  useAuthStore.getState().setAuth(user, accessToken);
}

const client = createClient<paths>({
  baseUrl: BASE,
});

// Auth header middleware
client.use({
  async onRequest({ request }) {
    request.credentials = "include";
    const token = useAuthStore.getState().token;
    if (token) {
      request.headers.set("Authorization", `Bearer ${token}`);
    }
    return request;
  },
  async onResponse({ response, request }) {
    // 401 → refresh → retry
    if (response.status === 401 && useAuthStore.getState().token) {
      if (!refreshPromise) {
        refreshPromise = refreshToken().finally(() => {
          refreshPromise = null;
        });
      }

      try {
        await refreshPromise;
      } catch {
        throw new ApiError(401, "UNAUTHORIZED");
      }

      // Retry with new token
      const newToken = useAuthStore.getState().token;
      if (newToken) {
        request.headers.set("Authorization", `Bearer ${newToken}`);
      }
      return fetch(request);
    }

    return response;
  },
});

export default client;
