import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";

vi.mock("@/api/campaignCreators", () => ({
  listCampaignCreators: vi.fn(),
}));

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
  getCreator: vi.fn(),
}));

import { listCampaignCreators } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import CampaignCreatorsSection from "./CampaignCreatorsSection";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";

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

function makeCC(creatorId: string) {
  return {
    id: `cc-${creatorId}`,
    campaignId: CAMPAIGN_ID,
    creatorId,
    status: "planned" as const,
    invitedAt: null,
    invitedCount: 0,
    remindedAt: null,
    remindedCount: 0,
    decidedAt: null,
    createdAt: "2026-05-07T12:00:00Z",
    updatedAt: "2026-05-07T12:00:00Z",
  };
}

function makeCreator(id: string, lastName: string) {
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
    socials: [{ platform: "instagram" as const, handle: lastName.toLowerCase() }],
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
  it("renders Spinner while A3 pending", () => {
    vi.mocked(listCampaignCreators).mockImplementation(
      () => new Promise(() => {}),
    );

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(screen.getByTestId("campaign-creators-section")).toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-table"),
    ).not.toBeInTheDocument();
  });

  it("renders ErrorState with retry on A3 failure; retry refires A3", async () => {
    vi.mocked(listCampaignCreators).mockRejectedValueOnce(new Error("boom"));
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("error-retry-button"));

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-table-empty"),
      ).toHaveTextContent("Креаторов пока нет");
    });
  });
});

describe("CampaignCreatorsSection — empty state", () => {
  it("renders empty message and disabled Add button when 0 creators", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    renderSection(FIXTURE_CAMPAIGN_LIVE);

    expect(
      await screen.findByTestId("campaign-creators-table-empty"),
    ).toHaveTextContent("Креаторов пока нет");

    const addBtn = screen.getByTestId("campaign-creators-add-button");
    expect(addBtn).toBeDisabled();
    expect(addBtn).toHaveAttribute("title", "Появится в следующем PR");
    expect(listCreators).not.toHaveBeenCalled();
    expect(
      screen.queryByTestId("campaign-creators-counter"),
    ).not.toBeInTheDocument();
  });
});

describe("CampaignCreatorsSection — happy path", () => {
  it("renders rows from listCreators, counter with total, Add disabled", async () => {
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
    expect(screen.getByTestId("campaign-creators-add-button")).toBeDisabled();
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
