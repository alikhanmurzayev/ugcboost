import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import RoleGuard from "./RoleGuard";
import { useAuthStore } from "@/stores/auth";
import { Roles } from "@/shared/constants/roles";

function renderWithRole(role: string) {
  useAuthStore.setState({
    user: { id: "1", email: "a@b.com", role: role as "admin" | "brand_manager" },
    token: "tok",
  });

  return render(
    <MemoryRouter initialEntries={["/admin"]}>
      <Routes>
        <Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
          <Route path="/admin" element={<div>Admin Content</div>} />
        </Route>
        <Route path="*" element={<div>Fallback</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("RoleGuard", () => {
  it("renders outlet for allowed role", () => {
    renderWithRole("admin");
    expect(screen.getByText("Admin Content")).toBeInTheDocument();
  });

  it("shows no-access for denied role", () => {
    renderWithRole("brand_manager");
    expect(screen.getByText("Нет доступа")).toBeInTheDocument();
  });
});
