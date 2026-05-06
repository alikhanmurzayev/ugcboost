import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import CampaignsListPage from "./CampaignsListPage";

vi.mock("@/api/campaigns", () => ({
  listCampaigns: vi.fn(),
}));

import { listCampaigns } from "@/api/campaigns";

const FIXTURE_LIVE = {
  id: "11111111-1111-1111-1111-111111111111",
  name: "Promo May",
  tmaUrl: "https://t.me/ugcboost_bot/app?startapp=promo-may",
  isDeleted: false,
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

const FIXTURE_DELETED = {
  id: "22222222-2222-2222-2222-222222222222",
  name: "Old Drop",
  tmaUrl: "https://t.me/ugcboost_bot/app?startapp=old-drop",
  isDeleted: true,
  createdAt: "2026-04-01T12:00:00Z",
  updatedAt: "2026-04-15T12:00:00Z",
};

function LocationProbe() {
  const loc = useLocation();
  return <div data-testid="probe-pathname">{loc.pathname + loc.search}</div>;
}

function renderPage(initialUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <Routes>
          <Route path="/campaigns" element={<CampaignsListPage />} />
          <Route
            path="/campaigns/new"
            element={<div data-testid="new-page">new</div>}
          />
          <Route
            path="/campaigns/:campaignId"
            element={<div data-testid="detail-page">detail</div>}
          />
        </Routes>
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CampaignsListPage — loading & error", () => {
  it("renders page container while loading", () => {
    vi.mocked(listCampaigns).mockImplementation(() => new Promise(() => {}));

    renderPage("/campaigns");

    expect(screen.getByTestId("campaigns-list-page")).toBeInTheDocument();
    expect(screen.queryByTestId("campaigns-total")).not.toBeInTheDocument();
  });

  it("renders error state on list failure", async () => {
    vi.mocked(listCampaigns).mockRejectedValueOnce(new Error("boom"));

    renderPage("/campaigns");

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
  });
});

describe("CampaignsListPage — empty states", () => {
  it("shows empty message when no items and no filters", async () => {
    vi.mocked(listCampaigns).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    await waitFor(() => {
      expect(screen.getByTestId("campaigns-table-empty")).toHaveTextContent(
        "Пока нет ни одной кампании",
      );
    });
  });

  it("shows filtered empty message when search active", async () => {
    vi.mocked(listCampaigns).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns?q=zzz");

    await waitFor(() => {
      expect(screen.getByTestId("campaigns-table-empty")).toHaveTextContent(
        "Нет кампаний по выбранным фильтрам",
      );
    });
  });
});

describe("CampaignsListPage — list rendering", () => {
  it("renders row by uuid and total counter", async () => {
    vi.mocked(listCampaigns).mockResolvedValueOnce({
      data: { items: [FIXTURE_LIVE], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    expect(
      await screen.findByTestId(`row-${FIXTURE_LIVE.id}`),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaigns-total")).toHaveTextContent("1");
  });

  it("renders sortable headers for name and createdAt", async () => {
    vi.mocked(listCampaigns).mockResolvedValueOnce({
      data: { items: [FIXTURE_LIVE], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    await screen.findByTestId(`row-${FIXTURE_LIVE.id}`);
    expect(screen.getByTestId("th-name")).toBeInTheDocument();
    expect(screen.getByTestId("th-createdAt")).toBeInTheDocument();
  });

  it("renders deleted badge for soft-deleted row", async () => {
    vi.mocked(listCampaigns).mockResolvedValueOnce({
      data: {
        items: [FIXTURE_LIVE, FIXTURE_DELETED],
        total: 2,
        page: 1,
        perPage: 50,
      },
    });

    renderPage("/campaigns?showDeleted=true");

    expect(
      await screen.findByTestId(`campaign-deleted-${FIXTURE_DELETED.id}`),
    ).toHaveTextContent("Удалена");
    expect(
      screen.queryByTestId(`campaign-deleted-${FIXTURE_LIVE.id}`),
    ).not.toBeInTheDocument();
  });
});

describe("CampaignsListPage — list body roundtrip", () => {
  it("calls list with default sort=created_at desc, page 1, isDeleted=false", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    await waitFor(() => {
      expect(listCampaigns).toHaveBeenCalledWith({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
        isDeleted: false,
      });
    });
  });

  it("omits isDeleted when ?showDeleted=true", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns?showDeleted=true");

    await waitFor(() => {
      expect(listCampaigns).toHaveBeenCalledWith({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      });
    });
  });

  it("forwards trimmed search from ?q", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns?q=promo");

    await waitFor(() => {
      expect(listCampaigns).toHaveBeenCalledWith(
        expect.objectContaining({ search: "promo", isDeleted: false }),
      );
    });
  });

  it("clicking name header toggles sort to asc", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const header = await screen.findByTestId("th-name");
    await userEvent.click(header);

    await waitFor(() => {
      expect(listCampaigns).toHaveBeenLastCalledWith(
        expect.objectContaining({ sort: "name", order: "asc" }),
      );
    });
  });

  it("toggling showDeleted writes URL and removes isDeleted param", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const checkbox = await screen.findByTestId("campaigns-show-deleted");
    await userEvent.click(checkbox);

    await waitFor(() => {
      expect(screen.getByTestId("probe-pathname").textContent).toContain(
        "showDeleted=true",
      );
    });
    await waitFor(() => {
      expect(listCampaigns).toHaveBeenLastCalledWith({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      });
    });
  });

  it("changing search resets page in URL", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 120, page: 2, perPage: 50 },
    });

    renderPage("/campaigns?page=2");

    const searchInput = await screen.findByTestId("campaigns-search");
    await userEvent.type(searchInput, "x");

    await waitFor(() => {
      expect(screen.getByTestId("probe-pathname").textContent).not.toContain(
        "page=",
      );
    });
  });
});

describe("CampaignsListPage — row navigation", () => {
  it("navigates to /campaigns/:id on row click", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const row = await screen.findByTestId(`row-${FIXTURE_LIVE.id}`);
    await userEvent.click(row);

    expect(await screen.findByTestId("detail-page")).toBeInTheDocument();
  });

  it("disabled delete renders with tooltip and click is a no-op", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const deleteBtn = await screen.findByTestId(
      `campaign-delete-${FIXTURE_LIVE.id}`,
    );
    expect(deleteBtn).toBeDisabled();
    expect(deleteBtn).toHaveAttribute("title", "Появится позже");

    await userEvent.click(deleteBtn);

    expect(screen.queryByTestId("detail-page")).not.toBeInTheDocument();
    expect(screen.getByTestId("campaigns-list-page")).toBeInTheDocument();
  });

  it("CTA button links to /campaigns/new", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const cta = await screen.findByTestId("campaigns-create-button");
    await userEvent.click(cta);

    expect(await screen.findByTestId("new-page")).toBeInTheDocument();
  });
});

describe("CampaignsListPage — pagination", () => {
  it("renders pagination when totalPages > 1", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 120, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    expect(await screen.findByTestId("pagination")).toBeInTheDocument();
    expect(screen.getByTestId("pagination-prev")).toBeDisabled();
    expect(screen.getByTestId("pagination-next")).not.toBeDisabled();
  });

  it("clicking next bumps page in URL", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      data: { items: [FIXTURE_LIVE], total: 120, page: 1, perPage: 50 },
    });

    renderPage("/campaigns");

    const next = await screen.findByTestId("pagination-next");
    await userEvent.click(next);

    await waitFor(() => {
      expect(listCampaigns).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 2 }),
      );
    });
  });
});
