import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@/shared/i18n/config";
import type { ReactNode } from "react";

vi.mock("@/api/campaignCreators", async () => {
  const actual = await vi.importActual<
    typeof import("@/api/campaignCreators")
  >("@/api/campaignCreators");
  return {
    ...actual,
    notifyCampaignCreators: vi.fn(),
    remindCampaignCreatorsInvitation: vi.fn(),
  };
});

import {
  notifyCampaignCreators,
  remindCampaignCreatorsInvitation,
  type CampaignCreator,
  type CampaignNotifyResult,
} from "@/api/campaignCreators";
import type { CreatorListItem } from "@/api/creators";
import { ApiError } from "@/api/client";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";
import { useCampaignNotifyMutations } from "./hooks/useCampaignNotifyMutations";
import CampaignCreatorGroupSection from "./CampaignCreatorGroupSection";
import type { CampaignCreatorRow } from "./hooks/useCampaignCreators";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const CREATOR_C = "cccccccc-cccc-cccc-cccc-cccccccccccc";

function makeCC(creatorId: string, status: CampaignCreator["status"] = "planned"): CampaignCreator {
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

interface HarnessProps {
  status: "planned" | "invited" | "declined" | "agreed";
  rows: CampaignCreatorRow[];
  actionLabel?: string;
  withMutation?: boolean;
  onRemove?: (row: CampaignCreatorRow) => void;
  onRowClick?: (row: CampaignCreatorRow) => void;
  drawerSelectedCreatorId?: string;
}

function Harness({
  status,
  rows,
  actionLabel,
  withMutation = true,
  onRemove,
  onRowClick,
  drawerSelectedCreatorId,
}: HarnessProps) {
  const { notify, remind } = useCampaignNotifyMutations(CAMPAIGN_ID);
  const mutation = withMutation
    ? status === "invited"
      ? remind
      : notify
    : undefined;
  return (
    <CampaignCreatorGroupSection
      status={status}
      campaignId={CAMPAIGN_ID}
      title={`group ${status}`}
      rows={rows}
      actionLabel={actionLabel}
      mutation={mutation}
      onRemove={onRemove ?? (() => {})}
      onRowClick={onRowClick ?? (() => {})}
      drawerSelectedCreatorId={drawerSelectedCreatorId}
    />
  );
}

function renderHarness(props: HarnessProps) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }
  const utils = render(<Harness {...props} />, { wrapper: Wrapper });
  return { ...utils, queryClient, invalidateSpy };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CampaignCreatorGroupSection — header + action visibility", () => {
  it("renders title with rows count", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    const header = screen.getByTestId("campaign-creators-group-planned");
    expect(within(header).getByRole("heading")).toHaveTextContent("group planned");
    expect(within(header).getByRole("heading")).toHaveTextContent("2");
  });

  it("does not render the action button when actionLabel is omitted (agreed group)", () => {
    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, "agreed"),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderHarness({ status: "agreed", rows, withMutation: false });

    expect(
      screen.queryByTestId("campaign-creators-group-action-agreed"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-creators-select-all-agreed"),
    ).not.toBeInTheDocument();
  });

  it("renders the action button when actionLabel + mutation are provided", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    const btn = screen.getByTestId("campaign-creators-group-action-planned");
    expect(btn).toHaveTextContent("Разослать приглашение");
  });
});

describe("CampaignCreatorGroupSection — selection state", () => {
  it("button is disabled while selection is empty", () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    expect(
      screen.getByTestId("campaign-creators-group-action-planned"),
    ).toBeDisabled();
  });

  it("checking one row enables button and turns select-all into indeterminate", async () => {
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

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

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

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

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

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

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    // Select all then click again to clear.
    await userEvent.click(screen.getByTestId("campaign-creators-select-all-planned"));
    await userEvent.click(screen.getByTestId("campaign-creators-select-all-planned"));

    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });
});

describe("CampaignCreatorGroupSection — submit + result parsing", () => {
  it("submitting fires mutation with selected creatorIds and renders inline-success when undelivered=[]", async () => {
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [] },
    } satisfies CampaignNotifyResult);

    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    const { invalidateSpy } = renderHarness({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
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

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned"),
      ).toBeInTheDocument();
    });

    expect(notifyCampaignCreators).toHaveBeenCalledWith(CAMPAIGN_ID, [
      CREATOR_A,
      CREATOR_B,
    ]);
    expect(
      screen.getByTestId("campaign-creators-group-result-planned"),
    ).toHaveTextContent("Доставлено 2");
    // Selection cleared, button disabled again.
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(
      screen.getByTestId("campaign-creators-group-action-planned"),
    ).toBeDisabled();
    // Invalidate fired with the campaign creators list key.
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignCreatorKeys.list(CAMPAIGN_ID),
    });
  });

  it("renders partial-success result with delivered count + undelivered list with name and reason", async () => {
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [{ creatorId: CREATOR_A, reason: "bot_blocked" }] },
    });

    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
      { campaignCreator: makeCC(CREATOR_B), creator: makeCreator(CREATOR_B, "Петрова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned"),
      ).toHaveTextContent("Доставлено 1");
    });
    const undelivered = screen.getByTestId(
      `campaign-creators-group-undelivered-planned-${CREATOR_A}`,
    );
    expect(undelivered).toHaveTextContent("Иванова Анна");
    expect(undelivered).toHaveTextContent("заблокировал(а) бота");
    // Selection cleared in onSettled even on partial-success.
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });

  it("undelivered with soft-deleted creator falls back to deletedPlaceholder", async () => {
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [{ creatorId: CREATOR_C, reason: "unknown" }] },
    });

    const rows: CampaignCreatorRow[] = [
      // Creator C row has no `creator` profile (soft-deleted).
      { campaignCreator: makeCC(CREATOR_C) },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_C}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId(
          `campaign-creators-group-undelivered-planned-${CREATOR_C}`,
        ),
      ).toHaveTextContent("—");
    });
  });

  it("422 CAMPAIGN_CREATOR_BATCH_INVALID renders inline validation error and clears selection", async () => {
    vi.mocked(notifyCampaignCreators).mockRejectedValueOnce(
      new ApiError(422, "CAMPAIGN_CREATOR_BATCH_INVALID", "batch invalid", [
        { creatorId: CREATOR_A, reason: "wrong_status", currentStatus: "invited" },
      ]),
    );

    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    const { invalidateSpy } = renderHarness({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned"),
      ).toHaveTextContent(/уже в другом статусе/i);
    });
    expect(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    ).not.toBeChecked();
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignCreatorKeys.list(CAMPAIGN_ID),
    });
  });

  it("network/5xx error renders inline networkError result", async () => {
    vi.mocked(notifyCampaignCreators).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );

    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-planned"),
    );

    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned"),
      ).toHaveTextContent(/Не удалось разослать/i);
    });
  });

  it("rapid double-click only fires the mutation once (double-submit guard)", async () => {
    let resolveCall: (v: CampaignNotifyResult) => void = () => {};
    vi.mocked(notifyCampaignCreators).mockImplementationOnce(
      () =>
        new Promise<CampaignNotifyResult>((r) => {
          resolveCall = r;
        }),
    );

    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderHarness({ status: "planned", rows, actionLabel: "Разослать приглашение" });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    const btn = screen.getByTestId("campaign-creators-group-action-planned");
    await userEvent.click(btn);
    await userEvent.click(btn);

    expect(notifyCampaignCreators).toHaveBeenCalledTimes(1);

    resolveCall({ data: { undelivered: [] } });
    await waitFor(() => {
      expect(
        screen.getByTestId("campaign-creators-group-result-planned"),
      ).toBeInTheDocument();
    });
  });

  it("invited group fires the remind mutation, not notify", async () => {
    vi.mocked(remindCampaignCreatorsInvitation).mockResolvedValueOnce({
      data: { undelivered: [] },
    });

    const rows: CampaignCreatorRow[] = [
      {
        campaignCreator: makeCC(CREATOR_A, "invited"),
        creator: makeCreator(CREATOR_A, "Иванова"),
      },
    ];

    renderHarness({ status: "invited", rows, actionLabel: "Разослать ремайндер" });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );
    await userEvent.click(
      screen.getByTestId("campaign-creators-group-action-invited"),
    );

    await waitFor(() => {
      expect(remindCampaignCreatorsInvitation).toHaveBeenCalledWith(
        CAMPAIGN_ID,
        [CREATOR_A],
      );
    });
    expect(notifyCampaignCreators).not.toHaveBeenCalled();
  });

  it("clicking a checkbox does not also fire row click (stopPropagation)", async () => {
    const onRowClick = vi.fn();
    const rows: CampaignCreatorRow[] = [
      { campaignCreator: makeCC(CREATOR_A), creator: makeCreator(CREATOR_A, "Иванова") },
    ];

    renderHarness({
      status: "planned",
      rows,
      actionLabel: "Разослать приглашение",
      onRowClick,
    });

    await userEvent.click(
      screen.getByTestId(`campaign-creator-checkbox-${CREATOR_A}`),
    );

    expect(onRowClick).not.toHaveBeenCalled();
  });
});
