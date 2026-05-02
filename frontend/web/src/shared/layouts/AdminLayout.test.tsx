import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import AdminLayout from "./AdminLayout";
import { useAuthStore } from "@/stores/auth";
import { Roles } from "@/shared/constants/roles";

vi.mock("@/api/creatorApplications", () => ({
  getCreatorApplicationsCounts: vi.fn(),
}));

vi.mock("@/api/auth", () => ({
  logout: vi.fn().mockResolvedValue(undefined),
}));

import { getCreatorApplicationsCounts } from "@/api/creatorApplications";

function setUser(role: "admin" | "brand_manager" | null) {
  if (role === null) {
    useAuthStore.setState({ user: null, token: null });
  } else {
    useAuthStore.setState({
      user: { id: "1", email: "alikhan@ugcboost.kz", role },
      token: "tok",
    });
  }
}

function renderLayout() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/creator-applications/verification"]}>
        <Routes>
          <Route element={<AdminLayout />}>
            <Route
              path="creator-applications/verification"
              element={<div>Verification stub</div>}
            />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AdminLayout", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nav and verification badge for admin", async () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: {
        items: [
          { status: "verification", count: 7 },
          { status: "moderation", count: 2 },
        ],
      },
    });

    renderLayout();

    expect(
      screen.getByTestId("nav-link-creator-applications/verification"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/moderation"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("nav-link-creators")).toBeInTheDocument();

    await waitFor(() => {
      expect(
        screen.getByTestId("nav-badge-creator-applications/verification"),
      ).toHaveTextContent("7");
    });
  });

  it("hides badge when counts query errors", async () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockRejectedValue(
      new Error("boom"),
    );

    renderLayout();

    await waitFor(() => {
      expect(getCreatorApplicationsCounts).toHaveBeenCalled();
    });

    expect(
      screen.queryByTestId("nav-badge-creator-applications/verification"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/verification"),
    ).toBeInTheDocument();
  });

  it("hides badge when verification count is zero (sparse miss)", async () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: { items: [{ status: "moderation", count: 1 }] },
    });

    renderLayout();

    await waitFor(() => {
      expect(getCreatorApplicationsCounts).toHaveBeenCalled();
    });

    expect(
      screen.queryByTestId("nav-badge-creator-applications/verification"),
    ).not.toBeInTheDocument();
  });

  it("does not query counts for non-admin", () => {
    setUser(Roles.BRAND_MANAGER);
    renderLayout();

    expect(getCreatorApplicationsCounts).not.toHaveBeenCalled();
  });

  it("renders logout button and user email", () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: { items: [] },
    });

    renderLayout();

    expect(screen.getByText("alikhan@ugcboost.kz")).toBeInTheDocument();
    expect(screen.getByTestId("logout-button")).toBeInTheDocument();
  });
});
