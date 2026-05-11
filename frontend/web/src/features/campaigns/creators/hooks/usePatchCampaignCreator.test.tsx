import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

vi.mock("@/api/campaignCreators", () => ({
  patchCampaignCreator: vi.fn(),
}));

import {
  patchCampaignCreator,
  type CampaignCreator,
} from "@/api/campaignCreators";
import { ApiError } from "@/api/client";
import { campaignCreatorKeys } from "@/shared/constants/queryKeys";
import { usePatchCampaignCreator } from "./usePatchCampaignCreator";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

function makeCC(overrides: Partial<CampaignCreator>): CampaignCreator {
  return {
    id: "cc-1",
    campaignId: CAMPAIGN_ID,
    creatorId: CREATOR_A,
    status: "signed",
    invitedCount: 1,
    remindedCount: 0,
    createdAt: "2026-05-11T12:00:00Z",
    updatedAt: "2026-05-11T12:00:00Z",
    ...overrides,
  };
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  return { qc, Wrapper };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("usePatchCampaignCreator", () => {
  it("on success updates the list cache with the fresh row and invalidates", async () => {
    const updated = makeCC({ ticketSentAt: "2026-05-11T13:00:00Z" });
    vi.mocked(patchCampaignCreator).mockResolvedValueOnce(updated);

    const { qc, Wrapper } = makeWrapper();
    const initial = [
      makeCC({ creatorId: CREATOR_A, ticketSentAt: null }),
      makeCC({ id: "cc-2", creatorId: CREATOR_B, ticketSentAt: null }),
    ];
    qc.setQueryData(campaignCreatorKeys.list(CAMPAIGN_ID), initial);

    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(
      () => usePatchCampaignCreator(CAMPAIGN_ID),
      { wrapper: Wrapper },
    );

    result.current.mutate({
      creatorId: CREATOR_A,
      patch: { ticketSent: true },
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(patchCampaignCreator).toHaveBeenCalledWith(
      CAMPAIGN_ID,
      CREATOR_A,
      { ticketSent: true },
    );

    // Optimistic write left the target row with a non-null ticketSentAt; the
    // untouched row stays the way it was.
    const cached = qc.getQueryData<CampaignCreator[]>(
      campaignCreatorKeys.list(CAMPAIGN_ID),
    );
    expect(cached).not.toBeUndefined();
    expect(cached?.[0]?.ticketSentAt).not.toBeNull();
    expect(cached?.[1]?.ticketSentAt).toBeNull();

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignCreatorKeys.list(CAMPAIGN_ID),
    });
  });

  it("on error rolls back the optimistic write and invokes the onError callback", async () => {
    const err = new ApiError(422, "CAMPAIGN_CREATOR_TICKET_SENT_BAD_STATUS");
    vi.mocked(patchCampaignCreator).mockRejectedValueOnce(err);

    const { qc, Wrapper } = makeWrapper();
    const initial = [makeCC({ creatorId: CREATOR_A, ticketSentAt: null })];
    qc.setQueryData(campaignCreatorKeys.list(CAMPAIGN_ID), initial);

    const onError = vi.fn();
    const { result } = renderHook(
      () => usePatchCampaignCreator(CAMPAIGN_ID, { onError }),
      { wrapper: Wrapper },
    );

    result.current.mutate({
      creatorId: CREATOR_A,
      patch: { ticketSent: true },
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    const cached = qc.getQueryData<CampaignCreator[]>(
      campaignCreatorKeys.list(CAMPAIGN_ID),
    );
    expect(cached).toEqual(initial);
    expect(onError).toHaveBeenCalledWith(err);
  });

  it("cold cache (no prior setQueryData) — mutate succeeds and still invalidates", async () => {
    const updated = makeCC({ ticketSentAt: "2026-05-11T14:00:00Z" });
    vi.mocked(patchCampaignCreator).mockResolvedValueOnce(updated);

    const { qc, Wrapper } = makeWrapper();
    // No qc.setQueryData(...) — the list cache is genuinely cold (e.g. user
    // toggled before the initial GET resolved). onMutate must not throw,
    // onSettled must still invalidate so the refetch syncs the row.
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(
      () => usePatchCampaignCreator(CAMPAIGN_ID),
      { wrapper: Wrapper },
    );

    result.current.mutate({
      creatorId: CREATOR_A,
      patch: { ticketSent: true },
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignCreatorKeys.list(CAMPAIGN_ID),
    });
  });

  it("unset (ticketSent=false) clears ticketSentAt in the cache optimistically", async () => {
    const updated = makeCC({ ticketSentAt: null });
    vi.mocked(patchCampaignCreator).mockResolvedValueOnce(updated);

    const { qc, Wrapper } = makeWrapper();
    qc.setQueryData(campaignCreatorKeys.list(CAMPAIGN_ID), [
      makeCC({
        creatorId: CREATOR_A,
        ticketSentAt: "2026-05-11T12:00:00Z",
      }),
    ]);

    const { result } = renderHook(
      () => usePatchCampaignCreator(CAMPAIGN_ID),
      { wrapper: Wrapper },
    );

    result.current.mutate({
      creatorId: CREATOR_A,
      patch: { ticketSent: false },
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    const cached = qc.getQueryData<CampaignCreator[]>(
      campaignCreatorKeys.list(CAMPAIGN_ID),
    );
    expect(cached?.[0]?.ticketSentAt).toBeNull();
  });
});
