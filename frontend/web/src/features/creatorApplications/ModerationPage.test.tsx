import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ModerationPage from "./ModerationPage";

vi.mock("@/api/creatorApplications", () => ({
  listCreatorApplications: vi.fn(),
  getCreatorApplication: vi.fn(),
  rejectApplication: vi.fn(),
}));

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: { type: "cities", items: [] },
  }),
}));

import {
  listCreatorApplications,
  getCreatorApplication,
} from "@/api/creatorApplications";

const FIXTURE_ITEM = {
  id: "22222222-2222-2222-2222-222222222222",
  status: "moderation" as const,
  lastName: "Петров",
  firstName: "Пётр",
  middleName: null,
  birthDate: "2008-01-01",
  city: { code: "ALA", name: "Алматы", sortOrder: 10 },
  categories: [],
  socials: [{ platform: "instagram" as const, handle: "petrov" }],
  telegramLinked: true,
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-05-02T08:00:00Z",
};

const FIXTURE_DETAIL = {
  ...FIXTURE_ITEM,
  iin: "080101300001",
  phone: "+77001112244",
  address: null,
  categoryOtherText: null,
  consents: [],
  telegramLink: {
    telegramUserId: 42,
    telegramUsername: "petrov",
    telegramFirstName: null,
    telegramLastName: null,
    linkedAt: "2026-05-01T10:00:00Z",
  },
  telegramBotUrl:
    "https://t.me/ugcboost_test_bot?start=22222222-2222-2222-2222-222222222222",
};

function renderPage(initialUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <ModerationPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ModerationPage — loading & error", () => {
  it("renders spinner while loading", () => {
    vi.mocked(listCreatorApplications).mockImplementation(
      () => new Promise(() => {}),
    );

    renderPage("/creator-applications/moderation");

    expect(
      screen.getByTestId("creator-applications-moderation-page"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("moderation-total")).not.toBeInTheDocument();
  });

  it("renders error state on list failure", async () => {
    vi.mocked(listCreatorApplications).mockRejectedValueOnce(new Error("boom"));

    renderPage("/creator-applications/moderation");

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
  });
});

describe("ModerationPage — empty states", () => {
  it("shows empty message when no items and no filters", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation");

    await waitFor(() => {
      expect(
        screen.getByTestId("applications-table-empty"),
      ).toHaveTextContent("Нет заявок в этой очереди");
    });
  });

  it("shows filtered empty message when filters active", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation?cities=ALA");

    await waitFor(() => {
      expect(
        screen.getByTestId("applications-table-empty"),
      ).toHaveTextContent("Нет заявок по выбранным фильтрам");
    });
  });
});

describe("ModerationPage — list rendering", () => {
  it("renders applications table with row testid by uuid", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation");

    expect(
      await screen.findByTestId(`row-${FIXTURE_ITEM.id}`),
    ).toBeInTheDocument();
    expect(screen.getByTestId("moderation-total")).toHaveTextContent("1");
  });

  it("renders city column and omits telegram column", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation");

    const row = await screen.findByTestId(`row-${FIXTURE_ITEM.id}`);
    expect(row).toHaveTextContent(FIXTURE_ITEM.city.name);
    expect(screen.queryByTestId("row-telegram-linked")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("row-telegram-not-linked"),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("th-city")).toBeInTheDocument();
  });
});

describe("ModerationPage — drawer toggle via URL", () => {
  it("opens drawer when row clicked and fetches detail", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage("/creator-applications/moderation");

    const row = await screen.findByTestId(`row-${FIXTURE_ITEM.id}`);
    await userEvent.click(row);

    await waitFor(() => {
      expect(getCreatorApplication).toHaveBeenCalledWith(FIXTURE_ITEM.id);
    });
    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
  });

  it("opens drawer when ?id present on mount", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage(`/creator-applications/moderation?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() => {
      expect(getCreatorApplication).toHaveBeenCalledWith(FIXTURE_ITEM.id);
    });
  });

  it("renders reject + disabled approve in drawer footer for moderation", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage(`/creator-applications/moderation?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("reject-button")).toBeInTheDocument();
    const approve = await screen.findByTestId("approve-button");
    expect(approve).toBeDisabled();
  });

  it("closes drawer when close button clicked", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage(`/creator-applications/moderation?id=${FIXTURE_ITEM.id}`);

    const close = await screen.findByTestId("drawer-close");
    await userEvent.click(close);

    await waitFor(() => {
      expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
    });
  });
});

describe("ModerationPage — filter URL → list body roundtrip", () => {
  it("calls list with statuses=moderation and default sort=updated_at asc", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation");

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({
          statuses: ["moderation"],
          sort: "updated_at",
          order: "asc",
          page: 1,
          perPage: 50,
        }),
      );
    });
  });

  it("respects sort/order from URL params", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation?sort=full_name&order=asc");

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({
          statuses: ["moderation"],
          sort: "full_name",
          order: "asc",
        }),
      );
    });
  });

  it("clicking hoursInStage header toggles sort to updated_at desc", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/moderation");

    const header = await screen.findByTestId("th-hoursInStage");
    await userEvent.click(header);

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenLastCalledWith(
        expect.objectContaining({
          statuses: ["moderation"],
          sort: "updated_at",
          order: "desc",
        }),
      );
    });
  });

  it("forwards filter fields from URL to body", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage(
      "/creator-applications/moderation?dateFrom=2026-04-01&cities=ALA&q=Пётр",
    );

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({
          statuses: ["moderation"],
          search: "Пётр",
          dateFrom: "2026-04-01T00:00:00.000Z",
          cities: ["ALA"],
        }),
      );
    });
  });
});
