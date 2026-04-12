import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import AuthGuard from "./AuthGuard";
import { useAuthStore } from "@/stores/auth";

vi.mock("@/api/auth", () => ({
  restoreSession: vi.fn().mockResolvedValue(null),
}));

function renderWithRouter(user: ReturnType<typeof useAuthStore.getState>["user"]) {
  useAuthStore.setState({ user, token: user ? "tok" : null });

  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route path="/login" element={<div>Login Page</div>} />
        <Route element={<AuthGuard />}>
          <Route path="/" element={<div>Dashboard</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("AuthGuard", () => {
  it("redirects to login when no user", async () => {
    renderWithRouter(null);
    expect(await screen.findByText("Login Page")).toBeInTheDocument();
  });

  it("renders outlet when user exists", () => {
    renderWithRouter({ id: "1", email: "a@b.com", role: "admin" as const });
    expect(screen.getByText("Dashboard")).toBeInTheDocument();
  });
});
