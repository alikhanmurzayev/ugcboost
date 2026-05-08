import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

vi.mock("@/api/campaignCreators", () => ({
  notifyCampaignCreators: vi.fn(),
  remindCampaignCreatorsInvitation: vi.fn(),
}));

import {
  notifyCampaignCreators,
  remindCampaignCreatorsInvitation,
} from "@/api/campaignCreators";
import { ApiError } from "@/api/client";
import { useCampaignNotifyMutations } from "./useCampaignNotifyMutations";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useCampaignNotifyMutations", () => {
  it("notify.mutate calls notifyCampaignCreators with bound campaignId and the creatorIds variable", async () => {
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [] },
    });

    const { result } = renderHook(
      () => useCampaignNotifyMutations(CAMPAIGN_ID),
      { wrapper: makeWrapper() },
    );

    result.current.notify.mutate([CREATOR_A, CREATOR_B]);

    await waitFor(() => {
      expect(result.current.notify.isSuccess).toBe(true);
    });
    expect(notifyCampaignCreators).toHaveBeenCalledTimes(1);
    expect(notifyCampaignCreators).toHaveBeenCalledWith(CAMPAIGN_ID, [
      CREATOR_A,
      CREATOR_B,
    ]);
    expect(result.current.notify.data).toEqual({ data: { undelivered: [] } });
  });

  it("remind.mutate calls remindCampaignCreatorsInvitation with the same shape", async () => {
    vi.mocked(remindCampaignCreatorsInvitation).mockResolvedValueOnce({
      data: { undelivered: [{ creatorId: CREATOR_A, reason: "bot_blocked" }] },
    });

    const { result } = renderHook(
      () => useCampaignNotifyMutations(CAMPAIGN_ID),
      { wrapper: makeWrapper() },
    );

    result.current.remind.mutate([CREATOR_A]);

    await waitFor(() => {
      expect(result.current.remind.isSuccess).toBe(true);
    });
    expect(remindCampaignCreatorsInvitation).toHaveBeenCalledWith(CAMPAIGN_ID, [
      CREATOR_A,
    ]);
    expect(result.current.remind.data?.data.undelivered).toEqual([
      { creatorId: CREATOR_A, reason: "bot_blocked" },
    ]);
  });

  it("notify.mutate surfaces ApiError on 422 batch-invalid via mutation.error", async () => {
    const apiErr = new ApiError(
      422,
      "CAMPAIGN_CREATOR_BATCH_INVALID",
      "batch invalid",
      [
        { creatorId: CREATOR_A, reason: "wrong_status", currentStatus: "invited" },
      ],
    );
    vi.mocked(notifyCampaignCreators).mockRejectedValueOnce(apiErr);

    const { result } = renderHook(
      () => useCampaignNotifyMutations(CAMPAIGN_ID),
      { wrapper: makeWrapper() },
    );

    result.current.notify.mutate([CREATOR_A]);

    await waitFor(() => {
      expect(result.current.notify.isError).toBe(true);
    });
    expect(result.current.notify.error).toBe(apiErr);
    expect(result.current.notify.error?.code).toBe(
      "CAMPAIGN_CREATOR_BATCH_INVALID",
    );
  });

  it("remind.mutate surfaces ApiError on network-style 500", async () => {
    const apiErr = new ApiError(500, "INTERNAL_ERROR");
    vi.mocked(remindCampaignCreatorsInvitation).mockRejectedValueOnce(apiErr);

    const { result } = renderHook(
      () => useCampaignNotifyMutations(CAMPAIGN_ID),
      { wrapper: makeWrapper() },
    );

    result.current.remind.mutate([CREATOR_A]);

    await waitFor(() => {
      expect(result.current.remind.isError).toBe(true);
    });
    expect(result.current.remind.error?.status).toBe(500);
  });

  it("notify and remind are independent — calling one does not flip the other", async () => {
    vi.mocked(notifyCampaignCreators).mockResolvedValueOnce({
      data: { undelivered: [] },
    });

    const { result } = renderHook(
      () => useCampaignNotifyMutations(CAMPAIGN_ID),
      { wrapper: makeWrapper() },
    );

    result.current.notify.mutate([CREATOR_A]);

    await waitFor(() => {
      expect(result.current.notify.isSuccess).toBe(true);
    });
    expect(result.current.remind.isIdle).toBe(true);
    expect(remindCampaignCreatorsInvitation).not.toHaveBeenCalled();
  });
});
