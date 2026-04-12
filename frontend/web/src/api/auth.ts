import type { components } from "./generated/schema";
import client, { ApiError, rawClient } from "./client";

export type User = components["schemas"]["User"];

function extractErrorCode(error: unknown): string {
  const e = error as { error?: { code?: string } };
  return e?.error?.code ?? "INTERNAL_ERROR";
}

export async function login(email: string, password: string) {
  const { data, error, response } = await client.POST("/auth/login", {
    body: { email, password },
  });
  if (error) {
    throw new ApiError(response.status, extractErrorCode(error));
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
    throw new ApiError(response.status, extractErrorCode(error));
  }
  return data;
}

// Singleton promise prevents double-fire from React strict mode.
let restorePromise: Promise<{ user: User; token: string } | null> | null = null;

export function restoreSession(): Promise<{ user: User; token: string } | null> {
  if (restorePromise) return restorePromise;

  restorePromise = (async () => {
    const { data, error } = await rawClient.POST("/auth/refresh");

    if (error || !data) return null;

    return {
      user: data.data.user,
      token: data.data.accessToken,
    };
  })();

  restorePromise.finally(() => {
    restorePromise = null;
  });

  return restorePromise;
}
