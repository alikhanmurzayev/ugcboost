import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";

vi.mock("@/api/campaignCreators", () => ({
  listCampaignCreators: vi.fn(),
  addCampaignCreators: vi.fn(),
  removeCampaignCreator: vi.fn(),
}));

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
  getCreator: vi.fn(),
}));

vi.mock("@/api/dictionaries", () => ({
  listDictionary: vi.fn().mockResolvedValue({
    data: { type: "cities", items: [] },
  }),
}));

import {
  listCampaignCreators,
  addCampaignCreators,
  removeCampaignCreator,
} from "@/api/campaignCreators";
import type { CampaignCreator } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem } from "@/api/creators";
import { ApiError } from "@/api/client";
import CampaignCreatorsSection from "./CampaignCreatorsSection";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

const FIXTURE_CAMPAIGN_LIVE = {
  id: CAMPAIGN_ID,
  name: "Spring Promo",
  tmaUrl: "https://t.me/x",
  isDeleted: false,
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
} as const;

const FIXTURE_CAMPAIGN_DELETED = {
  ...FIXTURE_CAMPAIGN_LIVE,
  isDeleted: true,
} as const;

function makeCC(creatorId: string): CampaignCreator {
  return {
    id: `cc-${creatorId}`,
    campaignId: CAMPAIGN_ID,
    creatorId,
    status: "planned",
    invitedAt: null,
    invitedCount: 0,
    remindedAt: null,
    remindedCount: 0,
    decidedAt: null,
    createdAt: "2026-05-07T12:00:00Z",
    updatedAt: "2026-05-07T12:00:00Z",
  };
}

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

function renderSection(
  campaign: typeof FIXTURE_CAMPAIGN_LIVE | typeof FIXTURE_CAMPAIGN_DELETED,
  initialUrl = `/campaigns/${campaign.id}`,
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <CampaignCreatorsSection campaign={campaign} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CampaignCreatorsSection — visibility gate", () => {
  it("returns null when campaign.isDeleted is true and never fires A3", () => {
    renderSection(FIXTURE_CAMPAIGN_DELETED);

    expect(
      screen.queryByTestId("campaign-creators-section"),
    ).not.toBeInTheDocument();
    expect(listCampaignCreators).not.toHaveBeenCalled();
  });
});

describe("CampaignCreatorsSection — loading & error", () => {
  it("renders Spinner with testid while A3 pending", () => {
    vi.mocked(listCampaignCreators).mockImplementation(
      () => new Promise(() => {}),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(screen.getByTestId("campaign-creators-section")).toBeInTheDocument();
    expect(
      screen.getByTestId("campaign-creators-loading"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-table"),
    ).not.toBeInTheDocument();
  });

  it("renders ErrorState with retry on A3 ApiError; retry refires A3 and re-counts the call", async () => {
    vi.mocked(listCampaignCreators).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(await screen.findByTestId("error-state")).toHaveTextContent(
      "Не удалось загрузить креаторов",
    );
    await userEvent.click(screen.getByTestId("error-retry-button"));

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-table-empty"),
      ).toHaveTextContent("Креаторов пока нет");
    });
    expect(listCampaignCreators).toHaveBeenCalledTimes(2);
  });
});

describe("CampaignCreatorsSection — empty state", () => {
  it("renders empty message and an enabled Add button when 0 creators", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(
      await screen.findByTestId("campaign-creators-table-empty"),
    ).toHaveTextContent("Креаторов пока нет");

    const addBtn = screen.getByTestId("campaign-creators-add-button");
    expect(addBtn).not.toBeDisabled();
    expect(listCreators).not.toHaveBeenCalled();
    expect(
      screen.queryByTestId("campaign-creators-counter"),
    ).not.toBeInTheDocument();
  });
});

describe("CampaignCreatorsSection — happy path", () => {
  it("renders rows from listCreators, counter with total, Add enabled", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(await screen.findByTestId(`row-${CREATOR_A}`)).toBeInTheDocument();
    expect(screen.getByTestId("campaign-creators-counter")).toHaveTextContent(
      "1 в кампании",
    );
    expect(screen.getByTestId("campaign-creators-add-button")).not.toBeDisabled();
  });

  it("clicking a row marks it data-selected (URL written via setSearchParams)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(await screen.findByTestId(`row-${CREATOR_A}`));

    await waitFor(() => {
      expect(screen.getByTestId(`row-${CREATOR_A}`)).toHaveAttribute(
        "data-selected",
        "true",
      );
    });
  });

  it("does not write creatorId when row's creator profile is missing (soft-deleted)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 200 },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(await screen.findByTestId(`row-${CREATOR_A}`));

    expect(screen.getByTestId(`row-${CREATOR_A}`)).toHaveAttribute(
      "data-selected",
      "false",
    );
  });

  it("marks row data-selected when ?creatorId is set on mount", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(
      FIXTURE_CAMPAIGN_LIVE,
      `/campaigns/${CAMPAIGN_ID}?creatorId=${CREATOR_A}`,
    );

    const row = await screen.findByTestId(`row-${CREATOR_A}`);
    expect(row).toHaveAttribute("data-selected", "true");
  });
});

describe("CampaignCreatorsSection — Add drawer integration", () => {
  it("clicking Add opens the drawer", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValue([]);
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId("campaign-creators-table-empty");
    await userEvent.click(screen.getByTestId("campaign-creators-add-button"));

    expect(
      await screen.findByTestId("add-creators-drawer-body"),
    ).toBeInTheDocument();
  });

  it("Cancel inside the drawer closes it", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValue([]);
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId("campaign-creators-table-empty");
    await userEvent.click(screen.getByTestId("campaign-creators-add-button"));
    await screen.findByTestId("add-creators-drawer-body");
    await userEvent.click(screen.getByTestId("add-creators-drawer-cancel"));

    await waitFor(() => {
      expect(
        screen.queryByTestId("add-creators-drawer-body"),
      ).not.toBeInTheDocument();
    });
  });
});

describe("CampaignCreatorsSection — Remove flow", () => {
  it("clicking trash opens RemoveCreatorConfirm with creator name", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );

    const dialog = await screen.findByTestId("remove-creator-confirm");
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByText(/Иванова Анна/)).toBeInTheDocument();
  });

  it("RemoveCreatorConfirm uses a soft-deleted-creator placeholder when the creator profile is missing", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, perPage: 200 },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );

    const dialog = await screen.findByTestId("remove-creator-confirm");
    expect(within(dialog).getByText(/Креатор удалён из системы/)).toBeInTheDocument();
  });

  it("trash click does not also fire onRowClick (data-selected stays false)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );

    expect(screen.getByTestId(`row-${CREATOR_A}`)).toHaveAttribute(
      "data-selected",
      "false",
    );
  });

  it("Confirm fires removeCampaignCreator and closes the dialog on success", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockResolvedValueOnce(undefined);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    await waitFor(() => {
      expect(removeCampaignCreator).toHaveBeenCalledWith(
        CAMPAIGN_ID,
        CREATOR_A,
      );
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
  });

  it("Cancel closes the dialog without calling the mutation", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-cancel"),
    );

    expect(removeCampaignCreator).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
  });

  it("404 race: dialog closes silently and list invalidates", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockRejectedValueOnce(
      new ApiError(404, "CAMPAIGN_CREATOR_NOT_FOUND"),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    await waitFor(() => {
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
  });

  it("422 with an unknown code falls back to the generic removeFailed alert", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockRejectedValueOnce(
      new ApiError(422, "SOME_OTHER_CODE"),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("remove-creator-confirm-error"),
      ).toHaveTextContent(/Не удалось удалить/i);
    });
  });

  it("422 CAMPAIGN_CREATOR_AGREED keeps dialog open with agreed-error", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockRejectedValueOnce(
      new ApiError(422, "CAMPAIGN_CREATOR_AGREED"),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("remove-creator-confirm-error"),
      ).toHaveTextContent(/удалить нельзя/i);
    });
    expect(
      screen.getByTestId("remove-creator-confirm"),
    ).toBeInTheDocument();
  });

  it("trash click on a different row is ignored while a previous remove is still pending", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A),
      makeCC(CREATOR_B),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Иванова"),
          makeCreator(CREATOR_B, "Петрова"),
        ],
        total: 2,
        page: 1,
        perPage: 200,
      },
    });
    let resolveDelete: (v: unknown) => void = () => {};
    vi.mocked(removeCampaignCreator).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveDelete = resolve;
        }),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    // While the A-mutation is in-flight, clicking the trash for B must NOT
    // re-target the dialog at B (otherwise the user sees B's name + spinner
    // belonging to A's mutation).
    await userEvent.click(
      screen.getByTestId(`campaign-creator-remove-${CREATOR_B}`),
    );

    expect(
      within(screen.getByTestId("remove-creator-confirm")).queryByText(
        /Петрова Анна/,
      ),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByTestId("remove-creator-confirm")).getByText(
        /Иванова Анна/,
      ),
    ).toBeInTheDocument();

    resolveDelete(undefined);
  });

  it("rapid double-confirm calls removeCampaignCreator exactly once (double-submit guard)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    let resolveDelete: (v: unknown) => void = () => {};
    vi.mocked(removeCampaignCreator).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveDelete = resolve;
        }),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    const submit = await screen.findByTestId("remove-creator-confirm-submit");
    await userEvent.click(submit);
    // Second click while the mutation is still pending — must be ignored.
    await userEvent.click(submit);

    await waitFor(() => {
      expect(submit).toBeDisabled();
    });
    expect(removeCampaignCreator).toHaveBeenCalledTimes(1);

    resolveDelete(undefined);
  });

  it("5xx keeps dialog open with generic error", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("remove-creator-confirm-error"),
      ).toHaveTextContent(/Не удалось удалить/i);
    });
  });
});

describe("CampaignCreatorsSection — pass-through to drawer", () => {
  it("passes existingCreatorIds to the drawer so members render disabled", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
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

    renderSection(FIXTURE_CAMPAIGN_LIVE);
    await screen.findByTestId(`row-${CREATOR_A}`);
    await userEvent.click(screen.getByTestId("campaign-creators-add-button"));

    await waitFor(() => {
      expect(
        screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
      ).toBeDisabled();
    });
    expect(
      screen.getByTestId(`drawer-row-added-badge-${CREATOR_A}`),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).not.toBeDisabled();
    // sanity: ensure addCampaignCreators import is available even though
    // this assertion path doesn't exercise it directly.
    expect(addCampaignCreators).toBeDefined();
  });
});
