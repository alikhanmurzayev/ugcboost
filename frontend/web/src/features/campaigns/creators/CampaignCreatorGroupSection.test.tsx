import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@/shared/i18n/config";

import type {
  CampaignCreator,
  CampaignCreatorStatus,
} from "@/api/campaignCreators";
import type { CreatorListItem } from "@/api/creators";
import CampaignCreatorGroupSection from "./CampaignCreatorGroupSection";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";
import type { SectionResult } from "./notifyResult";

const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const CREATOR_C = "cccccccc-cccc-cccc-cccc-cccccccccccc";

function makeCC(
  creatorId: string,
  status: CampaignCreatorStatus = "planned",
  overrides: Partial<CampaignCreator> = {},
): CampaignCreator {
  return {
    id: `cc-${creatorId}`,
    campaignId: "11111111-1111-1111-1111-111111111111",
    creatorId,
    status,
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

interface RenderOpts {
  status?: CampaignCreatorStatus;
  rows: CampaignCreatorRow[];
  actionLabel?: string;
  actionSubmittingLabel?: string;
  onSubmit?: (
    creatorIds: string[],
    namesSnapshot: Record<string, string>,
  ) => void;
  result?: SectionResult | null;
  isSubmitting?: boolean;
  isPending?: boolean;
  onRemove?: (row: CampaignCreatorRow) => void;
  onRowClick?: (row: CampaignCreatorRow) => void;
  drawerSelectedCreatorId?: string;
}

function renderGroup({
  status = "planned",
  rows,
  actionLabel,
  actionSubmittingLabel,
  onSubmit,
  result = null,
  isSubmitting = false,
  isPending = false,
  onRemove,
  onRowClick,
  drawerSelectedCreatorId,
}: RenderOpts) {
  return render(
    <CampaignCreatorGroupSection
      status={status}
      title={`group ${status}`}
      rows={rows}
      actionLabel={actionLabel}
      actionSubmittingLabel={actionSubmittingLabel}
      onSubmit={onSubmit}
      result={result}
      isSubmitting={isSubmitting}
      isPending={isPending}
      onRemove={onRemove ?? (() => {})}
      onRowClick={onRowClick ?? (() => {})}
      drawerSelectedCreatorId={drawerSelectedCreatorId}
    />,
  );
}

describe("CampaignCreatorGroupSection — header + action visibility", () => {
  it("renders title with rows count", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    const header = screen.getByTestId("campaign-creators-group-planned");
    expect(within(header).getByRole("heading")).toHaveTextContent(
      "group planned",
    );
    expect(within(header).getByRole("heading")).toHaveTextContent("2");
  });

  it("does not render the action button when actionLabel is omitted (agreed group)", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, "agreed"),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderGroup({ status: "agreed", rows });

    expect(
      screen.queryByTestId("campaign-creators-group-action-agreed"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-select-all-agreed"),
    ).not.toBeInTheDocument();
  });

  it("renders the action button when actionLabel + onSubmit are provided", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    const btn = screen.getByTestId("campaign-creators-group-action-planned");
    expect(btn).toHaveTextContent("Разослать приглашение");
  });

  it("button shows submittingLabel while isSubmitting=true", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      actionSubmittingLabel: "Отправка…",
      onSubmit: () => {},
      isSubmitting: true,
    });

    const btn = screen.getByTestId("campaign-creators-group-action-planned");
    expect(btn).toHaveTextContent("Отправка…");
    expect(btn).toBeDisabled();
  });
});

describe("CampaignCreatorGroupSection — selection state", () => {
  it("button is disabled while selection is empty", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    expect(
      screen.getByTestId("campaign-creators-group-action-planned"),
    ).toBeDisabled();
  });

  it("checking one row enables button and turns select-all into indeterminate", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );

    expect(
      screen.getByTestId("campaign-creators-group-action-planned"),
    ).toBeEnabled();
    const headerCb = screen.getByTestId(
      "campaign-creators-select-all-planned",
    ) as HTMLInputElement;
    expect(headerCb.indeterminate).toBe(true);
  });

  it("checking every row turns select-all into checked", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    );

    const headerCb = screen.getByTestId(
      "campaign-creators-select-all-planned",
    ) as HTMLInputElement;
    expect(headerCb).toBeChecked();
    expect(headerCb.indeterminate).toBe(false);
  });

  it("clicking select-all with empty selection picks every creator in the group", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    await userEvent.click(
      screen.getByTestId("campaign-creators-select-all-planned"),
    );

    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).toBeChecked();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    ).toBeChecked();
  });

  it("clicking select-all when everything is checked clears selection", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    await userEvent.click(screen.getByTestId("campaign-creators-select-all-planned"));
    await userEvent.click(screen.getByTestId("campaign-creators-select-all-planned"));

    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });

  it("toggling beyond cap (200) is ignored — extra rows stay unchecked", async () => {
    // 201 rows; user picks select-all → only 200 land in the selection.
    const rows: CampaignCreatorRow[] = Array.from({ length: 201 }, (_, i) => {
      const id = `00000000-0000-0000-0000-${String(i).padStart(12, "0")}`;
      return {
        campaignCreator: makeCC(id),
        creator: makeCreator(id, `Last${i}`),
      };
    });

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
    });

    await userEvent.click(
      screen.getByTestId("campaign-creators-select-all-planned"),
    );

    expect(
      screen.getByTestId(
        "campaign-creators-group-counter-planned",
      ),
    ).toHaveTextContent("Выбрано: 200 / 200");
    expect(
      screen.getByTestId("campaign-creators-group-cap-hint-planned"),
    ).toHaveTextContent(/Максимум 200/);
  });
});

describe("CampaignCreatorGroupSection — submit + props integration", () => {
  it("submitting fires onSubmit with selected creatorIds + namesSnapshot", async () => {
    const onSubmit = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit,
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0]?.[0]).toEqual([CREATOR_A, CREATOR_B]);
    expect(onSubmit.mock.calls[0]?.[1]).toEqual({
      [CREATOR_A]: "Иванова Анна",
      [CREATOR_B]: "Петрова Анна",
    });
  });

  it("rapid double-click only fires onSubmit once when isSubmitting flips on", async () => {
    const onSubmit = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    const { rerender } = renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit,
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    const btn = screen.getByTestId("campaign-creators-group-action-planned");
    await userEvent.click(btn);
    // Parent flipping isSubmitting=true should disable the button before
    // any second click can fire — emulate that by re-rendering with the flag.
    rerender(
      <CampaignCreatorGroupSection
        status="planned"
        title="group planned"
        rows={rows}
        actionLabel="Разослать приглашение"
        onSubmit={onSubmit}
        result={null}
        isSubmitting={true}
        isPending={false}
        onRemove={() => {}}
        onRowClick={() => {}}
      />,
    );
    await userEvent.click(btn);

    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("clears selection when isSubmitting flips from true to false", async () => {
    const onSubmit = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    const { rerender } = renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit,
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).toBeChecked();

    // Submit → parent says isSubmitting=true.
    rerender(
      <CampaignCreatorGroupSection
        status="planned"
        title="group planned"
        rows={rows}
        actionLabel="Разослать приглашение"
        onSubmit={onSubmit}
        result={null}
        isSubmitting={true}
        isPending={false}
        onRemove={() => {}}
        onRowClick={() => {}}
      />,
    );
    // ... and then back to false (mutation settled).
    rerender(
      <CampaignCreatorGroupSection
        status="planned"
        title="group planned"
        rows={rows}
        actionLabel="Разослать приглашение"
        onSubmit={onSubmit}
        result={{ kind: "success", deliveredCount: 1, undelivered: [] }}
        isSubmitting={false}
        isPending={false}
        onRemove={() => {}}
        onRowClick={() => {}}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
      ).not.toBeChecked();
    });
  });

  it("checkboxes are disabled when isSubmitting=true", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      isSubmitting: true,
    });

    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).toBeDisabled();
    expect(
      screen.getByTestId("campaign-creators-select-all-planned"),
    ).toBeDisabled();
  });

  it("clicking a checkbox does not also fire row click (stopPropagation)", async () => {
    const onRowClick = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      onRowClick,
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );

    expect(onRowClick).not.toHaveBeenCalled();
  });
});

describe("CampaignCreatorGroupSection — result rendering", () => {
  it("renders inline-success result with delivered count", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];
    const result: SectionResult = {
      kind: "success",
      deliveredCount: 2,
      undelivered: [],
      undeliveredNames: {},
    };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId("campaign-creators-group-result-planned-success"),
    ).toHaveTextContent("Доставлено 2");
  });

  it("renders partial-success result with delivered count + undelivered list with name and reason", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];
    const result: SectionResult = {
      kind: "success",
      deliveredCount: 1,
      undelivered: [{ creatorId: CREATOR_A, reason: "bot_blocked" }],
      undeliveredNames: { [CREATOR_A]: "Иванова Анна", [CREATOR_B]: "Петрова Анна" },
    };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    const successBlock = screen.getByTestId(
      "campaign-creators-group-result-planned-success",
    );
    expect(successBlock).toHaveTextContent("Доставлен 1");
    const undelivered = screen.getByTestId(
      `campaign-creators-group-undelivered-planned-${CREATOR_A}`,
    );
    expect(undelivered).toHaveTextContent("Иванова Анна");
    expect(undelivered).toHaveTextContent("заблокировал(а) бота");
  });

  it("undelivered with soft-deleted creator falls back to deletedPlaceholder", () => {
    const rows: CampaignCreatorRow[] = [
      // Soft-deleted creator (no `creator` profile) but still in the group.
      { campaignCreator: makeCC(CREATOR_C) },
    ];
    const result: SectionResult = {
      kind: "success",
      deliveredCount: 0,
      undelivered: [{ creatorId: CREATOR_C, reason: "unknown" }],
      undeliveredNames: { [CREATOR_C]: "—" },
    };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId(
        `campaign-creators-group-undelivered-planned-${CREATOR_C}`,
      ),
    ).toHaveTextContent("—");
  });

  it("422 batch-invalid result renders inline validation alert with details list", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];
    const result: SectionResult = {
      kind: "validation_error",
      validationDetails: [{ creatorId: CREATOR_A, currentStatus: "invited" }],
      detailNames: { [CREATOR_A]: "Иванова Анна" },
    };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId(
        "campaign-creators-group-result-planned-validation",
      ),
    ).toHaveTextContent(/уже в другом статусе/i);
    const detailItem = screen.getByTestId(
      `campaign-creators-group-validation-details-planned-${CREATOR_A}`,
    );
    expect(detailItem).toHaveTextContent("Иванова Анна");
    expect(detailItem).toHaveTextContent(/Приглашён/);
  });

  it("422 batch-invalid (chunk 18): currentStatus signing/signed/signing_declined render the new locale strings", () => {
    const CREATOR_X = "12345678-1234-1234-1234-123456789abc";
    const CREATOR_Y = "fedcba98-7654-3210-fedc-ba9876543210";
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_X), creator: makeCreator(CREATOR_X, "Петрова") },
      { campaignCreator: makeCC(CREATOR_Y), creator: makeCreator(CREATOR_Y, "Соколова") },
    ];
    const result: SectionResult = {
      kind: "validation_error",
      validationDetails: [
        { creatorId: CREATOR_A, currentStatus: "signing" },
        { creatorId: CREATOR_X, currentStatus: "signed" },
        { creatorId: CREATOR_Y, currentStatus: "signing_declined" },
      ],
      detailNames: {
        [CREATOR_A]: "Иванова Анна",
        [CREATOR_X]: "Петрова Анна",
        [CREATOR_Y]: "Соколова Анна",
      },
    };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    const detailA = screen.getByTestId(
      `campaign-creators-group-validation-details-planned-${CREATOR_A}`,
    );
    expect(detailA).toHaveTextContent("Иванова Анна");
    expect(detailA).toHaveTextContent(/Подписывает договор/);

    const detailX = screen.getByTestId(
      `campaign-creators-group-validation-details-planned-${CREATOR_X}`,
    );
    expect(detailX).toHaveTextContent("Петрова Анна");
    expect(detailX).toHaveTextContent(/Подписал\(а\) договор/);

    const detailY = screen.getByTestId(
      `campaign-creators-group-validation-details-planned-${CREATOR_Y}`,
    );
    expect(detailY).toHaveTextContent("Соколова Анна");
    expect(detailY).toHaveTextContent(/Отказал\(ась\) от договора/);
  });

  it("422 unknown-code result renders distinct validation message", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];
    const result: SectionResult = { kind: "validation_unknown" };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId(
        "campaign-creators-group-result-planned-validation-unknown",
      ),
    ).toHaveTextContent(/Некорректный запрос/i);
  });

  it("network-error result renders inline networkError alert", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];
    const result: SectionResult = { kind: "network_error" };

    renderGroup({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId("campaign-creators-group-result-planned-network"),
    ).toHaveTextContent(/Не удалось разослать/i);
  });

  it("section renders heading + result even with rows.length=0 (after success → group emptied)", () => {
    const result: SectionResult = {
      kind: "success",
      deliveredCount: 3,
      undelivered: [],
      undeliveredNames: {},
    };

    renderGroup({
      status: "planned",
      rows: [],
      actionLabel: "Разослать приглашение",
      onSubmit: () => {},
      result,
    });

    expect(
      screen.getByTestId("campaign-creators-group-result-planned-success"),
    ).toHaveTextContent("Доставлено 3");
    // Heading still visible — count is now 0.
    const header = screen.getByTestId("campaign-creators-group-planned");
    expect(within(header).getByRole("heading")).toHaveTextContent(/group planned/);
  });
});

describe("CampaignCreatorGroupSection — counter columns wiring", () => {
  it("forwards status='invited' so the table renders counter+timestamp cells", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, "invited", {
          invitedCount: 1,
          invitedAt: "2026-05-06T14:30:00Z",
          remindedCount: 0,
          remindedAt: null,
        }),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderGroup({ status: "invited", rows });

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
  });
});
