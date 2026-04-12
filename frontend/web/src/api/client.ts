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

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = useAuthStore.getState().token;

  const headers: Record<string, string> = {};

  if (options.headers) {
    const entries =
      options.headers instanceof Headers
        ? options.headers.entries()
        : Object.entries(options.headers);
    for (const [k, v] of entries) {
      headers[k] = v;
    }
  }

  if (options.body) {
    headers["Content-Type"] ??= "application/json";
  }

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res = await fetch(`${BASE}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  // Try refresh on 401
  if (res.status === 401 && token) {
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
    headers["Authorization"] = `Bearer ${newToken}`;
    res = await fetch(`${BASE}${path}`, {
      ...options,
      headers,
      credentials: "include",
    });
  }

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const err = body?.error as { code?: string } | undefined;
    throw new ApiError(res.status, err?.code ?? "INTERNAL_ERROR");
  }

  return res.json() as Promise<T>;
}
