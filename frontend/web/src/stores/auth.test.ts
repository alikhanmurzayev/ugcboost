import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "./auth";

describe("useAuthStore", () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, token: null });
  });

  it("starts with null user and token", () => {
    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.token).toBeNull();
  });

  it("setAuth sets user and token", () => {
    const user = { id: "1", email: "a@b.com", role: "admin" as const, createdAt: "2024-01-01" };
    useAuthStore.getState().setAuth(user, "tok123");

    const state = useAuthStore.getState();
    expect(state.user).toEqual(user);
    expect(state.token).toBe("tok123");
  });

  it("clearAuth resets to null", () => {
    const user = { id: "1", email: "a@b.com", role: "admin" as const, createdAt: "2024-01-01" };
    useAuthStore.getState().setAuth(user, "tok123");
    useAuthStore.getState().clearAuth();

    const state = useAuthStore.getState();
    expect(state.user).toBeNull();
    expect(state.token).toBeNull();
  });
});
