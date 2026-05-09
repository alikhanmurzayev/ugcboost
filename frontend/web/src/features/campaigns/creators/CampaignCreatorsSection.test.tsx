import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";

vi.mock("@/api/campaignCreators", async () => {
  const actual = await vi.importActual<
    typeof import("@/api/campaignCreators")
  >("@/api/campaignCreators");
  return {
    ...actual,
    listCampaignCreators: vi.fn(),
    addCampaignCreators: vi.fn(),
    removeCampaignCreator: vi.fn(),
    notifyCampaignCreators: vi.fn(),
    remindCampaignCreatorsInvitation: vi.fn(),
  };
});

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
  notifyCampaignCreators,
  remindCampaignCreatorsInvitation,
} from "@/api/campaignCreators";
import type {
  CampaignCreator,
  CampaignCreatorStatus,
} from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem } from "@/api/creators";
import { ApiError } from "@/api/client";
import CampaignCreatorsSection from "./CampaignCreatorsSection";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const CREATOR_C = "cccccccc-cccc-cccc-cccc-cccccccccccc";
const CREATOR_D = "dddddddd-dddd-dddd-dddd-dddddddddddd";

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

function makeCC(
  creatorId: string,
  status: CampaignCreatorStatus = "planned",
): CampaignCreator {
  return {
    id: `cc-${creatorId}`,
    campaignId: CAMPAIGN_ID,
    creatorId,
    status,
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
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
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
      screen.queryByTestId("campaign-creators-empty-all"),
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
        screen.getByTestId("campaign-creators-empty-all"),
      ).toBeInTheDocument();
    });
    expect(listCampaignCreators).toHaveBeenCalledTimes(2);
  });
});

describe("CampaignCreatorsSection — empty state", () => {
  it("renders empty-all message and an enabled Add button when total=0; no group sections rendered", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(
      await screen.findByTestId("campaign-creators-empty-all"),
    ).toHaveTextContent("Креаторов в кампании пока нет");

    const addBtn = screen.getByTestId("campaign-creators-add-button");
    expect(addBtn).not.toBeDisabled();
    expect(listCreators).not.toHaveBeenCalled();
    expect(
      screen.queryByTestId("campaign-creators-counter"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-group-planned"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-group-invited"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-group-declined"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-group-agreed"),
    ).not.toBeInTheDocument();
  });
});

describe("CampaignCreatorsSection — grouped rendering", () => {
  it("renders 4 groups in canonical order (planned → invited → declined → agreed) with the right action labels", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
      makeCC(CREATOR_B, "invited"),
      makeCC(CREATOR_C, "declined"),
      makeCC(CREATOR_D, "agreed"),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Иванова"),
          makeCreator(CREATOR_B, "Петрова"),
          makeCreator(CREATOR_C, "Сидорова"),
          makeCreator(CREATOR_D, "Орлова"),
        ],
        total: 4,
        page: 1,
        perPage: 200,
      },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId("campaign-creators-group-planned");
    const groups = screen
      .getAllByTestId(/^campaign-creators-group-(planned|invited|declined|agreed)$/);
    expect(groups.map((g) => g.dataset.testid)).toEqual([
      "campaign-creators-group-planned",
      "campaign-creators-group-invited",
      "campaign-creators-group-declined",
      "campaign-creators-group-agreed",
    ]);

    expect(
      screen.getByTestId("campaign-creators-group-action-planned"),
    ).toHaveTextContent("Разослать приглашение");
    expect(
      screen.getByTestId("campaign-creators-group-action-invited"),
    ).toHaveTextContent("Разослать ремайндер");
    expect(
      screen.getByTestId("campaign-creators-group-action-declined"),
    ).toHaveTextContent("Разослать приглашение");
    expect(
      screen.queryByTestId("campaign-creators-group-action-agreed"),
    ).not.toBeInTheDocument();
  });

  it("skips groups with zero rows but rest stays visible", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
      makeCC(CREATOR_B, "agreed"),
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

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId("campaign-creators-group-planned");
    expect(
      screen.queryByTestId("campaign-creators-group-invited"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-group-declined"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByTestId("campaign-creators-group-agreed"),
    ).toBeInTheDocument();
  });

  it("renders counter with total > 0 across all groups", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
      makeCC(CREATOR_B, "invited"),
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

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId(`row-${CREATOR_A}`);
    expect(screen.getByTestId("campaign-creators-counter")).toHaveTextContent(
      "2 в кампании",
    );
  });

  it("clicking a row in a group marks it data-selected via setSearchParams", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

    await screen.findByTestId("campaign-creators-empty-all");
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

    await screen.findByTestId("campaign-creators-empty-all");
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
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

  it("Confirm fires removeCampaignCreator and closes the dialog on success", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

  it("trash click does not also fire onRowClick (data-selected stays false)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

  it("trash button is not rendered for an agreed creator (UI gates removal upstream of backend)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "agreed"),
      makeCC(CREATOR_B, "planned"),
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

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId(`campaign-creator-remove-${CREATOR_B}`);
    expect(
      screen.queryByTestId(`campaign-creator-remove-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("404 race: dialog closes silently and list invalidates", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

  it("Cancel closes the dialog without calling the mutation", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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

    await waitFor(() => {
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
    expect(removeCampaignCreator).not.toHaveBeenCalled();
  });

  it("5xx keeps dialog open with generic removeFailed alert", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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
      ).toHaveTextContent(/не удалось удалить/i);
    });
    expect(screen.getByTestId("remove-creator-confirm")).toBeInTheDocument();
  });

  it("422 with an unknown code falls back to the generic removeFailed alert", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(removeCampaignCreator).mockRejectedValueOnce(
      new ApiError(422, "SOME_NEW_VALIDATION"),
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
      ).toHaveTextContent(/не удалось удалить/i);
    });
  });

  it("rapid double-confirm calls removeCampaignCreator exactly once (double-submit guard)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    let resolveMutate: (() => void) | undefined;
    vi.mocked(removeCampaignCreator).mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveMutate = resolve;
        }),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    const submit = await screen.findByTestId("remove-creator-confirm-submit");
    await userEvent.click(submit);
    await userEvent.click(submit);
    await userEvent.click(submit);

    expect(removeCampaignCreator).toHaveBeenCalledTimes(1);
    resolveMutate?.();
    await waitFor(() => {
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
  });

  it("trash click on a different row is ignored while a previous remove is still pending", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
      makeCC(CREATOR_B, "planned"),
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
    let resolveMutate: (() => void) | undefined;
    vi.mocked(removeCampaignCreator).mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveMutate = resolve;
        }),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );
    await userEvent.click(
      await screen.findByTestId("remove-creator-confirm-submit"),
    );

    // Confirm dialog must still be Anna's — Petrova's trash must be a no-op.
    await userEvent.click(
      screen.getByTestId(`campaign-creator-remove-${CREATOR_B}`),
    );
    expect(
      within(screen.getByTestId("remove-creator-confirm")).getByText(
        /Иванова Анна/,
      ),
    ).toBeInTheDocument();

    resolveMutate?.();
    await waitFor(() => {
      expect(
        screen.queryByTestId("remove-creator-confirm"),
      ).not.toBeInTheDocument();
    });
  });

  it("RemoveCreatorConfirm uses a soft-deleted-creator placeholder when the creator profile is missing", async () => {
    // listCampaignCreators returns A; listCreators returns no profile for A.
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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
});

describe("CampaignCreatorsSection — notify flow integration", () => {
  it("notify success in the planned group renders inline-success and clears selection", async () => {
    // Use mockResolvedValue (not Once): after onSettled invalidate the queries
    // refetch, and we want the group to keep rendering with the same data so
    // the inline-success block stays attached to a still-mounted group.
    vi.mocked(listCampaignCreators).mockResolvedValue([
      makeCC(CREATOR_A, "planned"),
    ]);
    vi.mocked(listCreators).mockResolvedValue({
      data: {
        items: [makeCreator(CREATOR_A, "Иванова")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [] },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    await waitFor(() => {
      expect(notifyCampaignCreators).toHaveBeenCalledWith(CAMPAIGN_ID, [
        CREATOR_A,
      ]);
    });
    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned-success"),
      ).toHaveTextContent("Доставлен 1");
    });
  });

  it("invited group fires remind, planned group fires notify (independent mutations from same hook)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValue([
      makeCC(CREATOR_A, "planned"),
      makeCC(CREATOR_B, "invited"),
    ]);
    vi.mocked(listCreators).mockResolvedValue({
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
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [] },
    });
    vi.mocked(remindCampaignCreatorsInvitation).mockResolvedValueOnce({
      data: { undelivered: [] },
    });

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    await screen.findByTestId(`campaign-creator-checkbox-${CREATOR_A}`);

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );
    await waitFor(() => {
      expect(notifyCampaignCreators).toHaveBeenCalledWith(CAMPAIGN_ID, [
        CREATOR_A,
      ]);
    });

    await userEvent.click(
      await screen.findByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-invited"),
    );

    await waitFor(() => {
      expect(remindCampaignCreatorsInvitation).toHaveBeenCalledWith(
        CAMPAIGN_ID,
        [CREATOR_B],
      );
    });
    expect(notifyCampaignCreators).toHaveBeenCalledTimes(1);
  });
});

describe("CampaignCreatorsSection — pass-through to drawer", () => {
  it("passes existingCreatorIds to the drawer so members render disabled", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A, "planned"),
    ]);
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
    expect(addCampaignCreators).toBeDefined();
  });
});
