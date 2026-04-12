import type { components } from "./generated/schema";
import client, { apiBase, ApiError } from "./client";

export type User = components["schemas"]["User"];

export async function login(email: string, password: string) {
  const { data, error, response } = await client.POST("/auth/login", {
    body: { email, password },
  });
  if (error) {
    const e = error as unknown as components["schemas"]["ErrorResponse"];
    throw new ApiError(response.status, e.error?.code ?? "INTERNAL_ERROR");
  }
  return data;
}

export async function logout() {
  const { response } = await client.POST("/auth/logout");
  if (!response.ok) {
    throw new ApiError(response.status, "INTERNAL_ERROR");
  }
}

export async function getMe() {
  const { data, error, response } = await client.GET("/auth/me");
  if (error) {
    const e = error as unknown as components["schemas"]["ErrorResponse"];
    throw new ApiError(response.status, e.error?.code ?? "INTERNAL_ERROR");
  }
  return data;
}

// Singleton promise prevents double-fire from React strict mode.
let restorePromise: Promise<{ user: User; token: string } | null> | null = null;

export function restoreSession(): Promise<{ user: User; token: string } | null> {
  if (restorePromise) return restorePromise;

  restorePromise = (async () => {
    const res = await fetch(`${apiBase}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });

    if (!res.ok) return null;

    const body = await res.json();
    return {
      user: body.data.user as User,
      token: body.data.accessToken as string,
    };
  })();

  restorePromise.finally(() => {
    restorePromise = null;
  });

  return restorePromise;
}
