import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route, useSearchParams } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";
import CreatorFilters from "./CreatorFilters";

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: {
      type: "cities",
      items: [{ code: "ALA", name: "Алматы", sortOrder: 1 }],
    },
  }),
}));

function ParamsObserver({ onParams }: { onParams: (sp: URLSearchParams) => void }) {
  const [sp] = useSearchParams();
  onParams(sp);
  return null;
}

function renderFilters(initialUrl: string, onParams = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <Routes>
          <Route
            path="*"
            element={
              <>
                <CreatorFilters />
                <ParamsObserver onParams={onParams} />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CreatorFilters — search", () => {
  it("writes search to URL on input change", async () => {
    const params = vi.fn();
    renderFilters("/creators", params);

    const input = screen.getByTestId("filters-search");
    await userEvent.type(input, "Анна");

    await waitFor(() => {
      const last = params.mock.calls.at(-1)?.[0] as URLSearchParams;
      expect(last.get("q")).toBe("Анна");
    });
  });
});

describe("CreatorFilters — popover toggle", () => {
  it("opens popover when filters-toggle clicked", async () => {
    renderFilters("/creators");

    expect(screen.queryByTestId("filters-popover")).not.toBeInTheDocument();
    await userEvent.click(screen.getByTestId("filters-toggle"));
    expect(screen.getByTestId("filters-popover")).toBeInTheDocument();
  });

  it("closes popover when Escape pressed", async () => {
    renderFilters("/creators");
    await userEvent.click(screen.getByTestId("filters-toggle"));

    expect(screen.getByTestId("filters-popover")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByTestId("filters-popover")).not.toBeInTheDocument();
    });
  });
});

describe("CreatorFilters — age fields", () => {
  it("writes ageFrom to URL", async () => {
    const params = vi.fn();
    renderFilters("/creators", params);

    await userEvent.click(screen.getByTestId("filters-toggle"));
    const ageFrom = screen.getByTestId("filter-age-from");
    await userEvent.type(ageFrom, "18");

    await waitFor(() => {
      const last = params.mock.calls.at(-1)?.[0] as URLSearchParams;
      expect(last.get("ageFrom")).toBe("18");
    });
  });
});

describe("CreatorFilters — telegram filter absent", () => {
  it("does not render telegram filter row", async () => {
    renderFilters("/creators");
    await userEvent.click(screen.getByTestId("filters-toggle"));
    expect(
      screen.queryByTestId("filter-telegram-linked"),
    ).not.toBeInTheDocument();
  });
});

describe("CreatorFilters — reset", () => {
  it("renders reset button when filters active and clears them", async () => {
    const params = vi.fn();
    renderFilters("/creators?q=анна&cities=ALA", params);

    const reset = screen.getByTestId("filters-reset");
    await userEvent.click(reset);

    await waitFor(() => {
      const last = params.mock.calls.at(-1)?.[0] as URLSearchParams;
      expect(last.has("q")).toBe(false);
      expect(last.has("cities")).toBe(false);
    });
  });

  it("does not render reset button when no filters", () => {
    renderFilters("/creators");
    expect(screen.queryByTestId("filters-reset")).not.toBeInTheDocument();
  });
});

describe("CreatorFilters — page reset on filter change", () => {
  it("resets page=1 in URL when search changes", async () => {
    const params = vi.fn();
    renderFilters("/creators?page=3", params);

    const input = screen.getByTestId("filters-search");
    await userEvent.type(input, "x");

    await waitFor(() => {
      const last = params.mock.calls.at(-1)?.[0] as URLSearchParams;
      expect(last.has("page")).toBe(false);
    });
  });
});
