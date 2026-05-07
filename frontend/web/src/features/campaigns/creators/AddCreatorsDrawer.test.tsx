import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";

vi.mock("@/api/campaignCreators", () => ({
  addCampaignCreators: vi.fn(),
}));

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
}));

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: { type: "cities", items: [] },
  }),
}));

import { addCampaignCreators } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem } from "@/api/creators";
import { ApiError } from "@/api/client";
import { campaignCreatorKeys, campaignKeys } from "@/shared/constants/queryKeys";
import AddCreatorsDrawer from "./AddCreatorsDrawer";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const CREATOR_C = "cccccccc-cccc-cccc-cccc-cccccccccccc";

function makeCreator(id: string, lastName: string): CreatorListItem {
  return {
    id,
    lastName,
    firstName: "Анна",
    middleName: null,
    iin: "070101400001",
    birthDate: "2007-01-01",
    phone: "+77001112255",
    city: { code: "ALA", name: "Алматы", sortOrder: 10 },
    categories: [{ code: "fashion", name: "Мода", sortOrder: 1 }],
    socials: [{ platform: "instagram", handle: lastName.toLowerCase() }],
    telegramUsername: lastName.toLowerCase(),
    createdAt: "2026-04-30T12:00:00Z",
    updatedAt: "2026-04-30T12:00:00Z",
  };
}

interface RenderOpts {
  existingCreatorIds?: Set<string>;
  onClose?: () => void;
  onAdded?: (added: unknown) => void;
  open?: boolean;
  cap?: number;
}

function renderDrawer(opts: RenderOpts = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onClose = opts.onClose ?? vi.fn();
  const onAdded = opts.onAdded ?? vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <AddCreatorsDrawer
        open={opts.open ?? true}
        campaignId={CAMPAIGN_ID}
        existingCreatorIds={opts.existingCreatorIds ?? new Set()}
        onClose={onClose}
        onAdded={onAdded}
        cap={opts.cap}
      />
    </QueryClientProvider>,
  );
  return { ...utils, queryClient, onClose, onAdded };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("AddCreatorsDrawer — rendering", () => {
  it("returns null when open=false (Drawer hides body)", async () => {
    renderDrawer({ open: false });

    expect(screen.queryByTestId("add-creators-drawer-body")).not.toBeInTheDocument();
    expect(listCreators).not.toHaveBeenCalled();
  });

  it("renders body with title and counter when open=true", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderDrawer();

    expect(
      await screen.findByTestId("add-creators-drawer-body"),
    ).toBeInTheDocument();
    expect(screen.getByText("Добавить креаторов")).toBeInTheDocument();
    expect(
      screen.getByTestId("add-creators-drawer-counter"),
    ).toHaveTextContent("Выбрано: 0 / 200");
  });

  it("fires listCreators with sort=created_at desc per_page=50 by default", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderDrawer();

    await waitFor(() => {
      expect(listCreators).toHaveBeenCalledTimes(1);
    });
    expect(listCreators).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      }),
    );
  });
});

describe("AddCreatorsDrawer — selection", () => {
  it("toggling a checkbox updates counter and submit label", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });

    renderDrawer();

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );

    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 1 / 200",
    );
    expect(screen.getByTestId("add-creators-drawer-submit")).toHaveTextContent(
      "Добавить (1)",
    );
  });

  it("submit is disabled when nothing is selected", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });

    renderDrawer();

    await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`);
    expect(screen.getByTestId("add-creators-drawer-submit")).toBeDisabled();
  });

  it("disables already-added members and shows badge", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Иванова"),
          makeCreator(CREATOR_B, "Петрова"),
        ],
        total: 2,
        page: 1,
        perPage: 50,
      },
    });

    renderDrawer({ existingCreatorIds: new Set([CREATOR_A]) });

    await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`);
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).toBeDisabled();
    expect(
      screen.getByTestId(`drawer-row-added-badge-${CREATOR_A}`),
    ).toHaveTextContent("Добавлен");
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).not.toBeDisabled();
  });
});

describe("AddCreatorsDrawer — close handlers", () => {
  it("Cancel button calls onClose", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });
    const onClose = vi.fn();
    renderDrawer({ onClose });

    await screen.findByTestId("add-creators-drawer-body");
    await userEvent.click(screen.getByTestId("add-creators-drawer-cancel"));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("backdrop click calls onClose", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });
    const onClose = vi.fn();
    renderDrawer({ onClose });

    await screen.findByTestId("add-creators-drawer-body");
    await userEvent.click(screen.getByTestId("drawer-backdrop"));

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("AddCreatorsDrawer — submit happy", () => {
  it("happy submit invalidates list cache, fires onAdded, calls onClose", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockResolvedValueOnce([
      {
        id: "cc-1",
        campaignId: CAMPAIGN_ID,
        creatorId: CREATOR_A,
        status: "planned",
        invitedAt: null,
        invitedCount: 0,
        remindedAt: null,
        remindedCount: 0,
        decidedAt: null,
        createdAt: "2026-05-07T12:00:00Z",
        updatedAt: "2026-05-07T12:00:00Z",
      },
    ]);

    const onClose = vi.fn();
    const onAdded = vi.fn();
    const { queryClient } = renderDrawer({ onClose, onAdded });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(addCampaignCreators).toHaveBeenCalledWith(CAMPAIGN_ID, [CREATOR_A]);
      expect(onClose).toHaveBeenCalledTimes(1);
    });
    expect(onAdded).toHaveBeenCalledTimes(1);
    // Invalidate the whole prefix so both `list` and `profiles` (keyed on
    // creator-ids tuple) refetch after add.
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: campaignCreatorKeys.all(),
      }),
    );
  });
});

describe("AddCreatorsDrawer — submit errors", () => {
  it("422 CREATOR_ALREADY_IN_CAMPAIGN keeps drawer open, shows alert, invalidates list, clears selection so the user can retry behind the cap", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Иванова"),
          makeCreator(CREATOR_B, "Петрова"),
          makeCreator(CREATOR_C, "Сидорова"),
        ],
        total: 3,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockRejectedValueOnce(
      new ApiError(422, "CREATOR_ALREADY_IN_CAMPAIGN"),
    );

    const onClose = vi.fn();
    const { queryClient } = renderDrawer({ onClose });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    );
    await userEvent.click(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_C}`),
    );
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 3 / 200",
    );

    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("add-creators-drawer-error")).toHaveTextContent(
        /уже в кампании/i,
      );
    });
    expect(onClose).not.toHaveBeenCalled();
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: campaignCreatorKeys.all(),
      }),
    );
    // Selection is cleared so the user is not stuck behind the cap with stale
    // checkboxes after parent refetch marks some rows as «Добавлен».
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 0 / 200",
    );
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });

  it("404 invalidates campaign detail and closes drawer silently (no inline alert)", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockRejectedValueOnce(
      new ApiError(404, "CAMPAIGN_NOT_FOUND"),
    );

    const onClose = vi.fn();
    const { queryClient } = renderDrawer({ onClose });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalledTimes(1);
    });
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: campaignKeys.detail(CAMPAIGN_ID),
      }),
    );
  });

  it("422 with an unknown code surfaces the server-provided message verbatim", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockRejectedValueOnce(
      new ApiError(
        422,
        "CAMPAIGN_CREATOR_IDS_DUPLICATES",
        "В списке встречаются дубликаты идентификаторов",
      ),
    );

    const onClose = vi.fn();
    renderDrawer({ onClose });

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("add-creators-drawer-error")).toHaveTextContent(
        /дубликаты идентификаторов/i,
      );
    });
    expect(onClose).not.toHaveBeenCalled();
  });

  it("422 without server message falls back to the generic addFailed alert", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockRejectedValueOnce(
      new ApiError(422, "CAMPAIGN_CREATOR_IDS_REQUIRED"),
    );

    renderDrawer();

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("add-creators-drawer-error")).toHaveTextContent(
        /Не удалось сохранить/i,
      );
    });
  });

  it("5xx shows generic alert and keeps drawer open with selection intact", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    vi.mocked(addCampaignCreators).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );

    const onClose = vi.fn();
    renderDrawer({ onClose });

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(screen.getByTestId("add-creators-drawer-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("add-creators-drawer-error")).toHaveTextContent(
        /Не удалось сохранить/i,
      );
    });
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 1 / 200",
    );
  });
});

describe("AddCreatorsDrawer — double submit guard", () => {
  it("submit/cancel are disabled while pending; addCampaignCreators is called exactly once even with rapid clicks", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });
    let resolveAdd: (v: unknown) => void = () => {};
    vi.mocked(addCampaignCreators).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveAdd = resolve;
        }),
    );

    renderDrawer();

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    const submit = screen.getByTestId("add-creators-drawer-submit");
    await userEvent.click(submit);
    // A second click is fired immediately while the mutation is still pending.
    await userEvent.click(submit);

    await waitFor(() => {
      expect(submit).toBeDisabled();
    });
    expect(screen.getByTestId("add-creators-drawer-cancel")).toBeDisabled();
    expect(addCampaignCreators).toHaveBeenCalledTimes(1);

    resolveAdd([]);
  });
});

describe("AddCreatorsDrawer — empty / loading", () => {
  it("renders empty message when list is empty without filters", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderDrawer();

    expect(
      await screen.findByTestId("add-creators-drawer-table-empty"),
    ).toHaveTextContent(/Нет креаторов/);
  });

  it("renders Spinner while listCreators is pending", () => {
    vi.mocked(listCreators).mockImplementation(() => new Promise(() => {}));

    renderDrawer();

    expect(screen.getByTestId("add-creators-drawer-body")).toBeInTheDocument();
    expect(
      screen.queryByTestId("add-creators-drawer-table"),
    ).not.toBeInTheDocument();
  });
});

describe("AddCreatorsDrawer — cap (cap=2 to exercise the visual)", () => {
  it("once selection size hits cap, hint is visible, unchecked rows are disabled", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Иванова"),
          makeCreator(CREATOR_B, "Петрова"),
          makeCreator(CREATOR_C, "Сидорова"),
        ],
        total: 3,
        page: 1,
        perPage: 50,
      },
    });

    renderDrawer({ cap: 2 });

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    );

    // Hint surfaces only at cap; unchecked rows go disabled; submit reflects N.
    expect(
      screen.getByTestId("add-creators-drawer-cap-hint"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_C}`),
    ).toBeDisabled();
    // The two checked rows must remain enabled so the user can deselect.
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).not.toBeDisabled();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).not.toBeDisabled();
  });

  it("hint is absent when below cap", async () => {
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 50,
      },
    });

    renderDrawer({ cap: 2 });

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );

    expect(
      screen.queryByTestId("add-creators-drawer-cap-hint"),
    ).not.toBeInTheDocument();
  });
});

describe("AddCreatorsDrawer — selection persists across pagination", () => {
  it("selecting on page 1, paginating to page 2, returning to page 1 keeps the selection", async () => {
    const PAGE_1_ITEMS = [
      makeCreator(CREATOR_A, "Иванова"),
      makeCreator(CREATOR_B, "Петрова"),
    ];
    const CREATOR_D = "dddddddd-dddd-dddd-dddd-dddddddddddd";
    const PAGE_2_ITEMS = [
      makeCreator(CREATOR_C, "Сидорова"),
      makeCreator(CREATOR_D, "Лебедева"),
    ];
    vi.mocked(listCreators).mockImplementation(async (input) => {
      const page = input.page ?? 1;
      return {
        data: {
          items: page === 1 ? PAGE_1_ITEMS : PAGE_2_ITEMS,
          total: 80, // 80/50 = 2 pages
          page,
          perPage: 50,
        },
      };
    });

    renderDrawer();

    await userEvent.click(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 1 / 200",
    );

    await userEvent.click(
      screen.getByTestId("add-creators-drawer-pagination-next"),
    );

    // Page 2 row is unchecked; counter still shows 1.
    expect(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_C}`),
    ).not.toBeChecked();
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 1 / 200",
    );

    await userEvent.click(
      screen.getByTestId("add-creators-drawer-pagination-prev"),
    );

    // Back on page 1 — A is still checked.
    expect(
      await screen.findByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).toBeChecked();
    expect(screen.getByTestId("add-creators-drawer-counter")).toHaveTextContent(
      "Выбрано: 1 / 200",
    );
  });
});
