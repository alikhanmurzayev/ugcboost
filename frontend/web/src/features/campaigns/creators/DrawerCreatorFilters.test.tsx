import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: {
      type: "cities",
      items: [{ code: "ALA", name: "Алматы", sortOrder: 1 }],
    },
  }),
}));

import DrawerCreatorFilters from "./DrawerCreatorFilters";
import type { FilterValues } from "@/features/creators/filters";

const EMPTY: FilterValues = {
  search: undefined,
  dateFrom: undefined,
  dateTo: undefined,
  cities: [],
  ageFrom: undefined,
  ageTo: undefined,
  categories: [],
};

type OnChangeMock = ReturnType<typeof vi.fn<[FilterValues], void>>;

interface RenderOpts {
  filters?: FilterValues;
  onChange?: OnChangeMock;
}

function renderFilters(opts: RenderOpts = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onChange: OnChangeMock = opts.onChange ?? vi.fn<[FilterValues], void>();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <DrawerCreatorFilters
        filters={opts.filters ?? EMPTY}
        onChange={onChange}
      />
    </QueryClientProvider>,
  );
  return { ...utils, onChange };
}

function lastChangeArg(mock: OnChangeMock): FilterValues {
  const last = mock.mock.calls.at(-1);
  if (!last) throw new Error("onChange was not called");
  return last[0];
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("DrawerCreatorFilters", () => {
  it("renders search input and filter toggle button", () => {
    renderFilters();

    expect(screen.getByTestId("drawer-filters-search")).toBeInTheDocument();
    expect(screen.getByTestId("drawer-filters-toggle")).toBeInTheDocument();
  });

  it("calls onChange with new search value when typing in the search input", async () => {
    const onChange = vi.fn<[FilterValues], void>();
    renderFilters({ onChange });

    await userEvent.type(screen.getByTestId("drawer-filters-search"), "А");

    expect(onChange).toHaveBeenCalled();
    expect(lastChangeArg(onChange).search).toBe("А");
  });

  it("opens popover when filter toggle is clicked and closes on Escape", async () => {
    renderFilters();

    expect(
      screen.queryByTestId("drawer-filters-popover"),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId("drawer-filters-toggle"));
    expect(screen.getByTestId("drawer-filters-popover")).toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    await waitFor(() => {
      expect(
        screen.queryByTestId("drawer-filters-popover"),
      ).not.toBeInTheDocument();
    });
  });

  it("renders reset button only when at least one filter is active", () => {
    const { rerender } = renderFilters();
    expect(
      screen.queryByTestId("drawer-filters-reset"),
    ).not.toBeInTheDocument();

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    rerender(
      <QueryClientProvider client={queryClient}>
        <DrawerCreatorFilters
          filters={{ ...EMPTY, search: "А" }}
          onChange={() => {}}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByTestId("drawer-filters-reset")).toBeInTheDocument();
  });

  it("reset button calls onChange with an empty filter object", async () => {
    const onChange = vi.fn<[FilterValues], void>();
    renderFilters({
      filters: { ...EMPTY, search: "А", cities: ["ALA"] },
      onChange,
    });

    await userEvent.click(screen.getByTestId("drawer-filters-reset"));

    expect(onChange).toHaveBeenCalledTimes(1);
    const arg = lastChangeArg(onChange);
    expect(arg.search).toBeUndefined();
    expect(arg.cities).toEqual([]);
    expect(arg.dateFrom).toBeUndefined();
    expect(arg.ageFrom).toBeUndefined();
  });

  it("ageFrom input change is forwarded through onChange (controlled — last keystroke wins on the last call)", async () => {
    const onChange = vi.fn<[FilterValues], void>();
    renderFilters({ onChange });

    await userEvent.click(screen.getByTestId("drawer-filters-toggle"));
    await userEvent.type(screen.getByTestId("drawer-filter-age-from"), "1");

    expect(onChange).toHaveBeenCalled();
    expect(lastChangeArg(onChange).ageFrom).toBe(1);
  });
});
