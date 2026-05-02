import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import RoleGuard from "./RoleGuard";
import { useAuthStore } from "@/stores/auth";
import { Roles } from "@/shared/constants/roles";

function renderWithRole(role: string | null) {
  if (role === null) {
    useAuthStore.setState({ user: null, token: null });
  } else {
    useAuthStore.setState({
      user: { id: "1", email: "a@b.com", role: role as "admin" | "brand_manager" },
      token: "tok",
    });
  }

  return render(
    <MemoryRouter initialEntries={["/admin"]}>
      <Routes>
        <Route path="/" element={<div>Home</div>} />
        <Route path="/login" element={<div>Login</div>} />
        <Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
          <Route path="/admin" element={<div>Admin Content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("RoleGuard", () => {
  it("renders outlet for allowed role", () => {
    renderWithRole("admin");
    expect(screen.getByText("Admin Content")).toBeInTheDocument();
  });

  it("redirects to dashboard for denied role", () => {
    renderWithRole("brand_manager");
    expect(screen.getByText("Home")).toBeInTheDocument();
    expect(screen.queryByText("Admin Content")).not.toBeInTheDocument();
  });

  it("redirects to login when no user", () => {
    renderWithRole(null);
    expect(screen.getByText("Login")).toBeInTheDocument();
    expect(screen.queryByText("Admin Content")).not.toBeInTheDocument();
  });
});
