import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import VerificationPage from "./VerificationPage";

vi.mock("@/api/creatorApplications", () => ({
  listCreatorApplications: vi.fn(),
  getCreatorApplication: vi.fn(),
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
  id: "11111111-1111-1111-1111-111111111111",
  status: "verification" as const,
  lastName: "Иванов",
  firstName: "Иван",
  middleName: null,
  birthDate: "2008-01-01",
  city: { code: "ALA", name: "Алматы", sortOrder: 10 },
  categories: [],
  socials: [
    { platform: "instagram" as const, handle: "ivan" },
  ],
  telegramLinked: true,
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

const FIXTURE_DETAIL = {
  ...FIXTURE_ITEM,
  iin: "080101300000",
  phone: "+77001112233",
  address: null,
  categoryOtherText: null,
  consents: [],
  telegramLink: null,
  telegramBotUrl:
    "https://t.me/ugcboost_test_bot?start=11111111-1111-1111-1111-111111111111",
};

function renderPage(initialUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <VerificationPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("VerificationPage — loading & error", () => {
  it("renders spinner while loading", () => {
    vi.mocked(listCreatorApplications).mockImplementation(
      () => new Promise(() => {}),
    );

    renderPage("/creator-applications/verification");

    expect(
      screen.getByTestId("creator-applications-verification-page"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("verification-total"),
    ).not.toBeInTheDocument();
  });

  it("renders error state on list failure", async () => {
    vi.mocked(listCreatorApplications).mockRejectedValueOnce(
      new Error("boom"),
    );

    renderPage("/creator-applications/verification");

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
  });
});

describe("VerificationPage — empty states", () => {
  it("shows empty message when no items and no filters", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification");

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

    renderPage("/creator-applications/verification?cities=ALA");

    await waitFor(() => {
      expect(
        screen.getByTestId("applications-table-empty"),
      ).toHaveTextContent("Нет заявок по выбранным фильтрам");
    });
  });
});

describe("VerificationPage — list rendering", () => {
  it("renders applications table with row testid by uuid", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification");

    expect(
      await screen.findByTestId(`row-${FIXTURE_ITEM.id}`),
    ).toBeInTheDocument();
    expect(screen.getByTestId("verification-total")).toHaveTextContent("1");
  });

  it("renders telegram-linked icon for linked row", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification");

    expect(
      await screen.findByTestId("row-telegram-linked"),
    ).toBeInTheDocument();
  });

  it("renders telegram-not-linked icon for unlinked row", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValueOnce({
      data: {
        items: [{ ...FIXTURE_ITEM, telegramLinked: false }],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });

    renderPage("/creator-applications/verification");

    expect(
      await screen.findByTestId("row-telegram-not-linked"),
    ).toBeInTheDocument();
  });
});

describe("VerificationPage — drawer toggle via URL", () => {
  it("opens drawer when row clicked and fetches detail", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage("/creator-applications/verification");

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

    renderPage(`/creator-applications/verification?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() => {
      expect(getCreatorApplication).toHaveBeenCalledWith(FIXTURE_ITEM.id);
    });
  });

  it("shows copy-bot-message button when telegram is not linked", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: { ...FIXTURE_DETAIL, telegramLink: null },
    });

    renderPage(`/creator-applications/verification?id=${FIXTURE_ITEM.id}`);

    expect(
      await screen.findByTestId("drawer-copy-bot-message"),
    ).toBeInTheDocument();
  });

  it("hides copy-bot-message button when telegram already linked", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: {
        ...FIXTURE_DETAIL,
        telegramLink: {
          telegramUserId: 42,
          telegramUsername: "ivan",
          telegramFirstName: null,
          telegramLastName: null,
          linkedAt: "2026-04-30T13:00:00Z",
        },
      },
    });

    renderPage(`/creator-applications/verification?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    expect(
      screen.queryByTestId("drawer-copy-bot-message"),
    ).not.toBeInTheDocument();
  });

  it("copies hardcoded message with bot URL on click", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });

    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: { ...FIXTURE_DETAIL, telegramLink: null },
    });

    renderPage(`/creator-applications/verification?id=${FIXTURE_ITEM.id}`);

    const btn = await screen.findByTestId("drawer-copy-bot-message");
    await userEvent.click(btn);

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledTimes(1);
    });
    const arg = writeText.mock.calls[0][0] as string;
    expect(arg).toContain(FIXTURE_DETAIL.telegramBotUrl);
    expect(arg).toContain("UGC boost");
    expect(arg).toContain("Telegram");
  });

  it("closes drawer when close button clicked", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreatorApplication).mockResolvedValue({
      data: FIXTURE_DETAIL,
    });

    renderPage(`/creator-applications/verification?id=${FIXTURE_ITEM.id}`);

    const close = await screen.findByTestId("drawer-close");
    await userEvent.click(close);

    await waitFor(() => {
      expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
    });
  });
});

describe("VerificationPage — filter URL → list body roundtrip", () => {
  it("calls list with statuses=verification and filter fields from URL", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage(
      "/creator-applications/verification?dateFrom=2026-04-01&cities=ALA&q=Иван",
    );

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledTimes(1);
    });

    expect(listCreatorApplications).toHaveBeenCalledWith({
      statuses: ["verification"],
      sort: "created_at",
      order: "desc",
      page: 1,
      perPage: 50,
      search: "Иван",
      dateFrom: "2026-04-01T00:00:00.000Z",
      cities: ["ALA"],
    });
  });

  it("uses default sort/page when URL params absent", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification");

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({
          statuses: ["verification"],
          sort: "created_at",
          order: "desc",
          page: 1,
          perPage: 50,
        }),
      );
    });
  });

  it("forwards telegramLinked=true from URL to body", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification?telegramLinked=true");

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({ telegramLinked: true }),
      );
    });
  });

  it("forwards telegramLinked=false from URL to body", async () => {
    vi.mocked(listCreatorApplications).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creator-applications/verification?telegramLinked=false");

    await waitFor(() => {
      expect(listCreatorApplications).toHaveBeenCalledWith(
        expect.objectContaining({ telegramLinked: false }),
      );
    });
  });
});
