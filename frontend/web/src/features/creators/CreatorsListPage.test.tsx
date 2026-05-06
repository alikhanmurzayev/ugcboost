import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import CreatorsListPage from "./CreatorsListPage";

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
  getCreator: vi.fn(),
}));

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: { type: "cities", items: [] },
  }),
}));

import { listCreators, getCreator } from "@/api/creators";

const FIXTURE_ITEM = {
  id: "33333333-3333-3333-3333-333333333333",
  lastName: "Иванова",
  firstName: "Анна",
  middleName: null,
  iin: "070101400001",
  birthDate: "2007-01-01",
  phone: "+77001112255",
  city: { code: "ALA", name: "Алматы", sortOrder: 10 },
  categories: [{ code: "fashion", name: "Мода", sortOrder: 1 }],
  socials: [{ platform: "instagram" as const, handle: "anna" }],
  telegramUsername: "anna",
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

const FIXTURE_DETAIL = {
  id: FIXTURE_ITEM.id,
  iin: FIXTURE_ITEM.iin,
  sourceApplicationId: "44444444-4444-4444-4444-444444444444",
  lastName: FIXTURE_ITEM.lastName,
  firstName: FIXTURE_ITEM.firstName,
  middleName: null,
  birthDate: FIXTURE_ITEM.birthDate,
  phone: FIXTURE_ITEM.phone,
  cityCode: "ALA",
  cityName: "Алматы",
  address: "ул. Абая 1",
  categoryOtherText: null,
  telegramUserId: 42,
  telegramUsername: FIXTURE_ITEM.telegramUsername,
  telegramFirstName: null,
  telegramLastName: null,
  socials: [
    {
      id: "s1",
      platform: "instagram" as const,
      handle: "anna",
      verified: true,
      createdAt: "2026-04-30T12:00:00Z",
    },
  ],
  categories: [{ code: "fashion", name: "Мода" }],
  createdAt: FIXTURE_ITEM.createdAt,
  updatedAt: FIXTURE_ITEM.updatedAt,
};

function renderPage(initialUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <CreatorsListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CreatorsListPage — loading & error", () => {
  it("renders page container while loading", () => {
    vi.mocked(listCreators).mockImplementation(() => new Promise(() => {}));

    renderPage("/creators");

    expect(screen.getByTestId("creators-list-page")).toBeInTheDocument();
    expect(screen.queryByTestId("creators-total")).not.toBeInTheDocument();
  });

  it("renders error state on list failure", async () => {
    vi.mocked(listCreators).mockRejectedValueOnce(new Error("boom"));

    renderPage("/creators");

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
  });
});

describe("CreatorsListPage — empty states", () => {
  it("shows empty message when no items and no filters", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    await waitFor(() => {
      expect(screen.getByTestId("creators-table-empty")).toHaveTextContent(
        "Пока нет одобренных креаторов",
      );
    });
  });

  it("shows filtered empty message when filters active", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creators?cities=ALA");

    await waitFor(() => {
      expect(screen.getByTestId("creators-table-empty")).toHaveTextContent(
        "Нет креаторов по выбранным фильтрам",
      );
    });
  });
});

describe("CreatorsListPage — list rendering", () => {
  it("renders row by uuid and total counter", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    expect(
      await screen.findByTestId(`row-${FIXTURE_ITEM.id}`),
    ).toBeInTheDocument();
    expect(screen.getByTestId("creators-total")).toHaveTextContent("1");
  });

  it("renders 7 columns: index, fullName, socials, categories, age, city, createdAt", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    await screen.findByTestId(`row-${FIXTURE_ITEM.id}`);
    expect(screen.getByTestId("th-fullName")).toBeInTheDocument();
    expect(screen.getByTestId("th-age")).toBeInTheDocument();
    expect(screen.getByTestId("th-city")).toBeInTheDocument();
    expect(screen.getByTestId("th-createdAt")).toBeInTheDocument();
  });
});

describe("CreatorsListPage — drawer toggle via URL", () => {
  it("opens drawer when row clicked and fetches detail", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockResolvedValue({ data: FIXTURE_DETAIL });

    renderPage("/creators");

    const row = await screen.findByTestId(`row-${FIXTURE_ITEM.id}`);
    await userEvent.click(row);

    await waitFor(() => {
      expect(getCreator).toHaveBeenCalledWith(FIXTURE_ITEM.id);
    });
    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
  });

  it("opens drawer when ?id present on mount, prefills from row before detail resolves", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockImplementation(() => new Promise(() => {}));

    renderPage(`/creators?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId("drawer-full-name")).toHaveTextContent(
        "Иванова Анна",
      ),
    );
    expect(screen.getByTestId("drawer-iin")).toHaveTextContent(FIXTURE_ITEM.iin);
  });

  it("renders detail-only fields after detail resolves", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockResolvedValue({ data: FIXTURE_DETAIL });

    renderPage(`/creators?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer-address")).toHaveTextContent(
      "ул. Абая 1",
    );
    expect(
      screen.getByTestId("drawer-source-application-id"),
    ).toHaveTextContent(FIXTURE_DETAIL.sourceApplicationId);
  });

  it("closes drawer when close button clicked", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockResolvedValue({ data: FIXTURE_DETAIL });

    renderPage(`/creators?id=${FIXTURE_ITEM.id}`);

    const close = await screen.findByTestId("drawer-close");
    await userEvent.click(close);

    await waitFor(() => {
      expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
    });
  });

  it("renders drawer error when getCreator fails, list keeps rendering", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockRejectedValueOnce(new Error("boom"));

    renderPage(`/creators?id=${FIXTURE_ITEM.id}`);

    expect(await screen.findByTestId("drawer-error")).toBeInTheDocument();
    expect(screen.getByTestId(`row-${FIXTURE_ITEM.id}`)).toBeInTheDocument();
  });

  it("disables prev/next when selectedId is not in current items", async () => {
    const orphanId = "99999999-9999-9999-9999-999999999999";
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockResolvedValue({
      data: { ...FIXTURE_DETAIL, id: orphanId },
    });

    renderPage(`/creators?id=${orphanId}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    expect(screen.getByTestId("drawer-prev")).toBeDisabled();
    expect(screen.getByTestId("drawer-next")).toBeDisabled();
  });
});

describe("CreatorsListPage — list body roundtrip", () => {
  it("calls list with default sort=full_name asc and page 1", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    await waitFor(() => {
      expect(listCreators).toHaveBeenCalledWith(
        expect.objectContaining({
          sort: "full_name",
          order: "asc",
          page: 1,
          perPage: 50,
        }),
      );
    });
  });

  it("clicking fullName header toggles sort to desc", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 1, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    const header = await screen.findByTestId("th-fullName");
    await userEvent.click(header);

    await waitFor(() => {
      expect(listCreators).toHaveBeenLastCalledWith(
        expect.objectContaining({
          sort: "full_name",
          order: "desc",
        }),
      );
    });
  });

  it("forwards filter fields from URL to body", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderPage("/creators?dateFrom=2026-04-01&cities=ALA&q=Анна&ageFrom=18");

    await waitFor(() => {
      expect(listCreators).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "Анна",
          dateFrom: "2026-04-01T00:00:00.000Z",
          cities: ["ALA"],
          ageFrom: 18,
        }),
      );
    });
  });
});

describe("CreatorsListPage — keyboard navigation race", () => {
  // fireEvent (not userEvent) is intentional: this test reproduces the
  // race where two ArrowRight events fire before React commits the
  // re-render between them. userEvent flushes React between events and
  // would hide the very bug we're guarding against.
  it("two consecutive ArrowRight presses navigate by two steps without dropping the second", async () => {
    const item1 = { ...FIXTURE_ITEM, id: "id-aaaa-1111", lastName: "Aaaa" };
    const item2 = { ...FIXTURE_ITEM, id: "id-bbbb-2222", lastName: "Bbbb" };
    const item3 = { ...FIXTURE_ITEM, id: "id-cccc-3333", lastName: "Cccc" };
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [item1, item2, item3], total: 3, page: 1, perPage: 50 },
    });
    vi.mocked(getCreator).mockImplementation(() => new Promise(() => {}));

    renderPage(`/creators?id=${item1.id}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId("drawer-prev")).toBeDisabled(),
    );

    fireEvent.keyDown(window, { key: "ArrowRight" });
    fireEvent.keyDown(window, { key: "ArrowRight" });

    await waitFor(() =>
      expect(screen.getByTestId("drawer-next")).toBeDisabled(),
    );
    await waitFor(() => expect(getCreator).toHaveBeenCalledWith(item3.id));
  });
});

describe("CreatorsListPage — pagination", () => {
  it("renders pagination when totalPages > 1", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 120, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    expect(await screen.findByTestId("pagination")).toBeInTheDocument();
    expect(screen.getByTestId("pagination-prev")).toBeDisabled();
    expect(screen.getByTestId("pagination-next")).not.toBeDisabled();
  });

  it("clicking next bumps page in URL", async () => {
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [FIXTURE_ITEM], total: 120, page: 1, perPage: 50 },
    });

    renderPage("/creators");

    const next = await screen.findByTestId("pagination-next");
    await userEvent.click(next);

    await waitFor(() => {
      expect(listCreators).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 2 }),
      );
    });
  });
});
