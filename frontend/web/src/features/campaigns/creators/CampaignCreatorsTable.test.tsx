import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@/shared/i18n/config";
import CampaignCreatorsTable from "./CampaignCreatorsTable";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

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

describe("CampaignCreatorsTable", () => {
  it("renders empty state with provided message when rows is empty", () => {
    render(
      <CampaignCreatorsTable
        rows={[]}
        onRowClick={() => {}}
        emptyMessage="Креаторов пока нет"
      />,
    );

    expect(
      screen.getByTestId("campaign-creators-table-empty"),
    ).toHaveTextContent("Креаторов пока нет");
    expect(screen.queryByTestId("campaign-creators-table")).not.toBeInTheDocument();
  });

  it("renders one row per creator using creatorId as key", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Aлексей") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Борис") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(screen.getByTestId(`row-${CREATOR_A}`)).toBeInTheDocument();
    expect(screen.getByTestId(`row-${CREATOR_B}`)).toBeInTheDocument();
  });

  it("invokes onRowClick with full row when a row is clicked", async () => {
    const onRowClick = vi.fn();
    const row: CampaignCreatorRow = {
      campaignCreator: makeCC(CREATOR_A),
      creator: makeCreator(CREATOR_A, "Aлексей"),
    };

    render(
      <CampaignCreatorsTable rows={[row]} onRowClick={onRowClick} emptyMessage="" />,
    );

    await userEvent.click(screen.getByTestId(`row-${CREATOR_A}`));

    expect(onRowClick).toHaveBeenCalledWith(row);
  });

  it("marks the row whose creator id matches selectedKey via data-selected", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Aлексей") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Борис") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        onRowClick={() => {}}
        selectedKey={CREATOR_A}
        emptyMessage=""
      />,
    );

    expect(screen.getByTestId(`row-${CREATOR_A}`)).toHaveAttribute(
      "data-selected",
      "true",
    );
    expect(screen.getByTestId(`row-${CREATOR_B}`)).toHaveAttribute(
      "data-selected",
      "false",
    );
  });

  it("renders placeholder cells with deleted-creator tooltip when creator profile is missing", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A) },
    ];

    render(
      <CampaignCreatorsTable rows={rows} onRowClick={() => {}} emptyMessage="" />,
    );

    const fullName = screen.getByTestId(
      `campaign-creator-deleted-${CREATOR_A}`,
    );
    expect(fullName).toHaveTextContent("—");
    expect(fullName).toHaveAttribute("title", "Креатор удалён из системы");
  });

  it("renders fullName and city for present creator", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable rows={rows} onRowClick={() => {}} emptyMessage="" />,
    );

    const row = screen.getByTestId(`row-${CREATOR_A}`);
    expect(row).toHaveTextContent("Иванова Анна");
    expect(row).toHaveTextContent("Алматы");
  });
});
