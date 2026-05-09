import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@/shared/i18n/config";
import type { CampaignCreator } from "@/api/campaignCreators";
import type { CreatorListItem } from "@/api/creators";
import CampaignCreatorsTable from "./CampaignCreatorsTable";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

function makeCC(
  creatorId: string,
  overrides: Partial<CampaignCreator> = {},
): CampaignCreator {
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
    ...overrides,
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

describe("CampaignCreatorsTable", () => {
  it("renders empty state with provided message when rows is empty", () => {
    render(
      <CampaignCreatorsTable
        rows={[]}
        status="planned"
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
        status="planned"
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
      <CampaignCreatorsTable
        rows={[row]}
        status="planned"
        onRowClick={onRowClick}
        emptyMessage=""
      />,
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
        status="planned"
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
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    const fullName = screen.getByTestId(
      `campaign-creator-deleted-${CREATOR_A}`,
    );
    expect(fullName).toHaveTextContent("—");
    expect(fullName).toHaveAttribute("title", "Креатор удалён из системы");
  });

  it("does not render the actions column when onRemove is not provided", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.queryByTestId(`campaign-creator-remove-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("renders trash button per row when onRemove is provided", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        onRemove={() => {}}
        emptyMessage=""
      />,
    );

    const trashA = screen.getByTestId(`campaign-creator-remove-${CREATOR_A}`);
    expect(trashA).toBeInTheDocument();
    expect(trashA).toHaveAttribute(
      "aria-label",
      "Удалить креатора из кампании",
    );
    expect(
      screen.getByTestId(`campaign-creator-remove-${CREATOR_B}`),
    ).toBeInTheDocument();
  });

  it("clicking trash invokes onRemove with row and stops propagation (no row click)", async () => {
    const onRemove = vi.fn();
    const onRowClick = vi.fn();
    const row: CampaignCreatorRow = {
      campaignCreator: makeCC(CREATOR_A),
      creator: makeCreator(CREATOR_A, "Иванова"),
    };

    render(
      <CampaignCreatorsTable
        rows={[row]}
        status="planned"
        onRowClick={onRowClick}
        onRemove={onRemove}
        emptyMessage=""
      />,
    );

    await userEvent.click(
      screen.getByTestId(`campaign-creator-remove-${CREATOR_A}`),
    );

    expect(onRemove).toHaveBeenCalledTimes(1);
    expect(onRemove).toHaveBeenCalledWith(row);
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it("clicking the row outside the trash still fires onRowClick when onRemove is provided", async () => {
    const onRemove = vi.fn();
    const onRowClick = vi.fn();
    const row: CampaignCreatorRow = {
      campaignCreator: makeCC(CREATOR_A),
      creator: makeCreator(CREATOR_A, "Иванова"),
    };

    render(
      <CampaignCreatorsTable
        rows={[row]}
        status="planned"
        onRowClick={onRowClick}
        onRemove={onRemove}
        emptyMessage=""
      />,
    );

    await userEvent.click(screen.getByTestId(`row-${CREATOR_A}`));

    expect(onRowClick).toHaveBeenCalledTimes(1);
    expect(onRemove).not.toHaveBeenCalled();
  });

  it("renders concrete values in every column for a present creator", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    const row = screen.getByTestId(`row-${CREATOR_A}`);
    const cells = within(row).getAllByRole("cell");
    expect(cells).toHaveLength(7);

    expect(cells[0]).toHaveTextContent("1");
    expect(cells[1]).toHaveTextContent("Иванова Анна");
    const social = within(cells[2] as HTMLElement).getByTestId(
      "social-instagram",
    );
    expect(social).toHaveAttribute("href", "https://instagram.com/иванова");
    expect(social).toHaveTextContent("@иванова");
    expect(cells[3]).toHaveTextContent("Мода");
    const expectedAge = String(calcAgeAtToday("2007-01-01"));
    expect(cells[4]).toHaveTextContent(expectedAge);
    expect(cells[5]).toHaveTextContent("Алматы");
    expect(cells[6]).toHaveTextContent("30 апр.");
  });

  it("renders placeholder + tooltip in every column when creator profile is missing", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A) },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    const row = screen.getByTestId(`row-${CREATOR_A}`);
    const cells = within(row).getAllByRole("cell");
    expect(cells[0]).toHaveTextContent("1");
    for (const cell of cells.slice(1)) {
      expect(cell).toHaveTextContent("—");
      const placeholderSpan = within(cell as HTMLElement).getByTitle(
        "Креатор удалён из системы",
      );
      expect(placeholderSpan).toBeInTheDocument();
    }
  });
});

describe("CampaignCreatorsTable — selection", () => {
  it("does not render checkbox column when checkedCreatorIds is undefined", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.queryByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("renders header checkbox + per-row checkbox when checkedCreatorIds is provided", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
        checkedCreatorIds={new Set([CREATOR_A])}
        selectAllState="indeterminate"
        selectAllTestId="campaign-creators-select-all-planned"
      />,
    );

    expect(
      screen.getByTestId("campaign-creators-select-all-planned"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).toBeChecked();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });

  it("toggling a row checkbox calls onToggleOne with the creatorId and does not fire row click", async () => {
    const onToggleOne = vi.fn();
    const onRowClick = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={onRowClick}
        emptyMessage=""
        checkedCreatorIds={new Set()}
        onToggleOne={onToggleOne}
        selectAllState="unchecked"
      />,
    );

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );

    expect(onToggleOne).toHaveBeenCalledWith(CREATOR_A);
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it("toggling the header checkbox calls onToggleAll", async () => {
    const onToggleAll = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
        checkedCreatorIds={new Set()}
        onToggleAll={onToggleAll}
        selectAllState="unchecked"
        selectAllTestId="campaign-creators-select-all-planned"
      />,
    );

    await userEvent.click(
      screen.getByTestId("campaign-creators-select-all-planned"),
    );

    expect(onToggleAll).toHaveBeenCalledTimes(1);
  });

  it("header checkbox.indeterminate is set as HTML property when state is indeterminate", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
        checkedCreatorIds={new Set([CREATOR_A])}
        selectAllState="indeterminate"
        selectAllTestId="campaign-creators-select-all-planned"
      />,
    );

    const headerCb = screen.getByTestId(
      "campaign-creators-select-all-planned",
    ) as HTMLInputElement;
    expect(headerCb.indeterminate).toBe(true);
    expect(headerCb).not.toBeChecked();
  });

  it("header checkbox is checked when selectAllState='checked'", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
        checkedCreatorIds={new Set([CREATOR_A])}
        selectAllState="checked"
        selectAllTestId="campaign-creators-select-all-planned"
      />,
    );

    const headerCb = screen.getByTestId(
      "campaign-creators-select-all-planned",
    ) as HTMLInputElement;
    expect(headerCb).toBeChecked();
    expect(headerCb.indeterminate).toBe(false);
  });

  it("clicking a row checkbox does not toggle row data-selected (stopPropagation)", async () => {
    const onRowClick = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={onRowClick}
        emptyMessage=""
        checkedCreatorIds={new Set()}
        onToggleOne={() => {}}
        selectAllState="unchecked"
      />,
    );

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );

    expect(onRowClick).not.toHaveBeenCalled();
    expect(screen.getByTestId(`row-${CREATOR_A}`)).toHaveAttribute(
      "data-selected",
      "false",
    );
  });
});

describe("CampaignCreatorsTable — counter columns by status", () => {
  it("planned: no counter/timestamp cells in DOM", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="planned"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.queryByTestId(`campaign-creator-invited-count-${CREATOR_A}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`campaign-creator-invited-at-${CREATOR_A}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`campaign-creator-reminded-at-${CREATOR_A}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`campaign-creator-decided-at-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("invited (после первого notify): composite cells render count·timestamp; 0-remindedCount and null remindedAt render literal '0' and em-dash", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "invited",
          invitedCount: 1,
          invitedAt: "2026-05-06T14:30:00Z",
          remindedCount: 0,
          remindedAt: null,
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="invited"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    // Lock i18n key resolution: a typo in `campaignCreators.columns.*` would
    // surface here as the raw key, not the translated header.
    expect(
      screen.getByRole("columnheader", { name: "Приглашение" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Ремайндер" }),
    ).toBeInTheDocument();

    expect(
      screen.getByTestId(`campaign-creator-invited-count-${CREATOR_A}`),
    ).toHaveTextContent("1");
    expect(
      screen.getByTestId(`campaign-creator-invited-at-${CREATOR_A}`),
    ).toHaveTextContent(/6 мая/);
    expect(
      screen.getByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).toHaveTextContent("0");
    expect(
      screen.getByTestId(`campaign-creator-reminded-at-${CREATOR_A}`),
    ).toHaveTextContent("—");
    // No decided column for invited.
    expect(
      screen.queryByTestId(`campaign-creator-decided-at-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("invited (после remind): reminded cells reflect new count + non-empty timestamp", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "invited",
          invitedCount: 1,
          invitedAt: "2026-05-06T14:30:00Z",
          remindedCount: 2,
          remindedAt: "2026-05-07T11:00:00Z",
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="invited"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).toHaveTextContent("2");
    expect(
      screen.getByTestId(`campaign-creator-reminded-at-${CREATOR_A}`),
    ).toHaveTextContent(/7 мая/);
  });

  it("declined: composite invited cell + decided cell render; reminded columns absent", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "declined",
          invitedCount: 2,
          invitedAt: "2026-05-06T14:30:00Z",
          decidedAt: "2026-05-08T10:00:00Z",
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="declined"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByRole("columnheader", { name: "Приглашение" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Решение" }),
    ).toBeInTheDocument();

    expect(
      screen.getByTestId(`campaign-creator-invited-count-${CREATOR_A}`),
    ).toHaveTextContent("2");
    // composite-ячейка теперь показывает и invitedAt в declined/agreed.
    expect(
      screen.getByTestId(`campaign-creator-invited-at-${CREATOR_A}`),
    ).toHaveTextContent(/6 мая/);
    expect(
      screen.getByTestId(`campaign-creator-decided-at-${CREATOR_A}`),
    ).toHaveTextContent(/8 мая/);
    expect(
      screen.queryByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId(`campaign-creator-reminded-at-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("agreed: same layout as declined — invitedCount + decidedAt only", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "agreed",
          invitedCount: 1,
          invitedAt: "2026-05-06T14:30:00Z",
          decidedAt: "2026-05-08T10:00:00Z",
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="agreed"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`campaign-creator-invited-count-${CREATOR_A}`),
    ).toHaveTextContent("1");
    expect(
      screen.getByTestId(`campaign-creator-decided-at-${CREATOR_A}`),
    ).toHaveTextContent(/8 мая/);
    expect(
      screen.queryByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).not.toBeInTheDocument();
  });

  it("invited: invalid invitedAt ISO falls back to em-dash without crash", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "invited",
          invitedCount: 1,
          invitedAt: "not-a-date",
          remindedCount: 0,
          remindedAt: null,
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="invited"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`campaign-creator-invited-at-${CREATOR_A}`),
    ).toHaveTextContent("—");
  });

  it("invited: deleted creator (no profile) still gets counter/timestamp from campaignCreator data", () => {
    // Counter и timestamp ячейки берут данные из row.campaignCreator, который
    // существует даже когда профиль креатора soft-deleted. data-testid
    // привязан к creatorId — должен собраться без падения.
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, {
          status: "invited",
          invitedCount: 1,
          invitedAt: "2026-05-06T14:30:00Z",
          remindedCount: 0,
          remindedAt: null,
        }),
      },
    ];

    render(
      <CampaignCreatorsTable
        rows={rows}
        status="invited"
        onRowClick={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`campaign-creator-invited-count-${CREATOR_A}`),
    ).toHaveTextContent("1");
    expect(
      screen.getByTestId(`campaign-creator-invited-at-${CREATOR_A}`),
    ).toHaveTextContent(/6 мая/);
    expect(
      screen.getByTestId(`campaign-creator-reminded-count-${CREATOR_A}`),
    ).toHaveTextContent("0");
    expect(
      screen.getByTestId(`campaign-creator-reminded-at-${CREATOR_A}`),
    ).toHaveTextContent("—");
    // ФИО-ячейка показывает deleted-плейсхолдер, как и раньше.
    expect(
      screen.getByTestId(`campaign-creator-deleted-${CREATOR_A}`),
    ).toBeInTheDocument();
  });
});

function calcAgeAtToday(birthDate: string): number {
  const birth = new Date(birthDate);
  const now = new Date();
  let age = now.getFullYear() - birth.getFullYear();
  const m = now.getMonth() - birth.getMonth();
  if (m < 0 || (m === 0 && now.getDate() < birth.getDate())) age--;
  return age;
}
