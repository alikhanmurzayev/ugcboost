import type { components } from "./generated/schema";
import { api, apiBase } from "./client";

export type User = components["schemas"]["User"];
type LoginResult = components["schemas"]["LoginResult"];
type UserResponse = components["schemas"]["UserResponse"];

export function login(email: string, password: string) {
  return api<LoginResult>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function logout() {
  return api<components["schemas"]["MessageResponse"]>("/auth/logout", {
    method: "POST",
  });
}

export function getMe() {
  return api<UserResponse>("/auth/me");
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

    const body = (await res.json()) as LoginResult;
    return {
      user: body.data.user,
      token: body.data.accessToken,
    };
  })();

  restorePromise.finally(() => {
    restorePromise = null;
  });

  return restorePromise;
}
