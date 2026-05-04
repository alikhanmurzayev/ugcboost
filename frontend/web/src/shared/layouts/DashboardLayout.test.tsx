import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import DashboardLayout from "./DashboardLayout";
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

function renderLayout(initialUrl = "/") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <Routes>
          <Route element={<DashboardLayout />}>
            <Route index element={<div>Dashboard stub</div>} />
            <Route
              path="creator-applications/verification"
              element={<div>Verification stub</div>}
            />
            <Route
              path="creator-applications/moderation"
              element={<div>Moderation stub</div>}
            />
            <Route path="brands" element={<div>Brands stub</div>} />
            <Route path="audit" element={<div>Audit stub</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("DashboardLayout — admin nav", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders all admin nav links incl. dashboard, applications, creators, brands, audit", async () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: { items: [] },
    });

    renderLayout();

    expect(screen.getByTestId("nav-link-/")).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/verification"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/moderation"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/contracts"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/rejected"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("nav-link-creators")).toBeInTheDocument();
    expect(screen.getByTestId("nav-link-brands")).toBeInTheDocument();
    expect(screen.getByTestId("nav-link-audit")).toBeInTheDocument();
  });

  it("shows verification badge from counts", async () => {
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

    await waitFor(() => {
      expect(
        screen.getByTestId("nav-badge-creator-applications/verification"),
      ).toHaveTextContent("7");
    });
  });

  it("shows moderation badge from counts", async () => {
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

    await waitFor(() => {
      expect(
        screen.getByTestId("nav-badge-creator-applications/moderation"),
      ).toHaveTextContent("2");
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
      screen.queryByTestId("nav-badge-creator-applications/moderation"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/verification"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("nav-link-creator-applications/moderation"),
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

  it("hides moderation badge when count is zero (sparse miss)", async () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: { items: [{ status: "verification", count: 3 }] },
    });

    renderLayout();

    await waitFor(() => {
      expect(getCreatorApplicationsCounts).toHaveBeenCalled();
    });

    expect(
      screen.queryByTestId("nav-badge-creator-applications/moderation"),
    ).not.toBeInTheDocument();
  });
});

describe("DashboardLayout — brand manager nav", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders only dashboard and brands for brand_manager", () => {
    setUser(Roles.BRAND_MANAGER);
    renderLayout();

    expect(screen.getByTestId("nav-link-/")).toBeInTheDocument();
    expect(screen.getByTestId("nav-link-brands")).toBeInTheDocument();
    expect(
      screen.queryByTestId("nav-link-creator-applications/verification"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("nav-link-creators"),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("nav-link-audit")).not.toBeInTheDocument();
  });

  it("does not query counts for brand_manager", () => {
    setUser(Roles.BRAND_MANAGER);
    renderLayout();

    expect(getCreatorApplicationsCounts).not.toHaveBeenCalled();
  });
});

describe("DashboardLayout — chrome", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders user email and logout button", () => {
    setUser(Roles.ADMIN);
    vi.mocked(getCreatorApplicationsCounts).mockResolvedValue({
      data: { items: [] },
    });

    renderLayout();

    expect(screen.getByText("alikhan@ugcboost.kz")).toBeInTheDocument();
    expect(screen.getByTestId("logout-button")).toBeInTheDocument();
  });
});
