import { api } from "./client";

interface User {
  id: string;
  email: string;
  role: "admin" | "brand_manager";
}

interface LoginResponse {
  data: {
    accessToken: string;
    user: User;
  };
}

interface UserResponse {
  data: User;
}

export function login(email: string, password: string) {
  return api<LoginResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function logout() {
  return api<{ data: { message: string } }>("/auth/logout", {
    method: "POST",
  });
}

export function getMe() {
  return api<UserResponse>("/auth/me");
}

// restoreSession tries to get a new access token via the refresh cookie,
// then fetches the current user. Used on page reload when token is lost.
export async function restoreSession(): Promise<{
  user: User;
  token: string;
} | null> {
  const BASE = "/api";

  const res = await fetch(`${BASE}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });

  if (!res.ok) return null;

  const body = (await res.json()) as LoginResponse;
  return {
    user: body.data.user,
    token: body.data.accessToken,
  };
}
