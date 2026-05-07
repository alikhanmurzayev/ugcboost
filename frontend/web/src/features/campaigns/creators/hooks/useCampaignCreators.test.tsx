import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

vi.mock("@/api/campaignCreators", () => ({
  listCampaignCreators: vi.fn(),
}));

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
}));

import { listCampaignCreators } from "@/api/campaignCreators";
import type { CampaignCreator } from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import type { CreatorListItem } from "@/api/creators";
import { useCampaignCreators } from "./useCampaignCreators";

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";

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

function wrap() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useCampaignCreators", () => {
  it("returns empty rows and skips listCreators when campaign has no creators", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.rows).toEqual([]);
    expect(result.current.total).toBe(0);
    expect(listCreators).not.toHaveBeenCalled();
  });

  it("composes A3 + listCreators({ids}) and joins by creator id", async () => {
    const ccA = makeCC(CREATOR_A);
    const ccB = makeCC(CREATOR_B);
    const creatorA = makeCreator(CREATOR_A, "Aлексей");
    const creatorB = makeCreator(CREATOR_B, "Борис");

    vi.mocked(listCampaignCreators).mockResolvedValueOnce([ccA, ccB]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [creatorB, creatorA], total: 2, page: 1, perPage: 200 },
    });

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.rows).toHaveLength(2);
    });

    expect(listCreators).toHaveBeenCalledTimes(1);
    expect(listCreators).toHaveBeenCalledWith({
      ids: [CREATOR_A, CREATOR_B].sort(),
      page: 1,
      perPage: 2,
      sort: "created_at",
      order: "desc",
    });

    expect(result.current.total).toBe(2);
    expect(result.current.rows[0]).toEqual({
      campaignCreator: ccB,
      creator: creatorB,
    });
    expect(result.current.rows[1]).toEqual({
      campaignCreator: ccA,
      creator: creatorA,
    });
  });

  it("appends rows for creators missing from listCreators (soft-deleted) without creator profile", async () => {
    const ccA = makeCC(CREATOR_A);
    const ccB = makeCC(CREATOR_B);
    const creatorA = makeCreator(CREATOR_A, "Aлексей");

    vi.mocked(listCampaignCreators).mockResolvedValueOnce([ccA, ccB]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: { items: [creatorA], total: 1, page: 1, perPage: 200 },
    });

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.rows).toHaveLength(2);
    });

    expect(result.current.rows[0]).toEqual({
      campaignCreator: ccA,
      creator: creatorA,
    });
    expect(result.current.rows[1]).toEqual({ campaignCreator: ccB });
    expect(result.current.rows[1].creator).toBeUndefined();
  });

  it("surfaces error when A3 fails", async () => {
    vi.mocked(listCampaignCreators).mockRejectedValueOnce(new Error("boom"));

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(listCreators).not.toHaveBeenCalled();
  });

  it("surfaces error when listCreators fails after A3 succeeded", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([makeCC(CREATOR_A)]);
    vi.mocked(listCreators).mockRejectedValueOnce(new Error("boom"));

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });

  it("does not fire A3 when enabled is false", () => {
    renderHook(
      () => useCampaignCreators(CAMPAIGN_ID, { enabled: false }),
      { wrapper: wrap() },
    );

    expect(listCampaignCreators).not.toHaveBeenCalled();
    expect(listCreators).not.toHaveBeenCalled();
  });

  it("exposes existingCreatorIds as a Set built from A3 creator ids", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A),
      makeCC(CREATOR_B),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [
          makeCreator(CREATOR_A, "Aлексей"),
          makeCreator(CREATOR_B, "Борис"),
        ],
        total: 2,
        page: 1,
        perPage: 200,
      },
    });

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.existingCreatorIds).toBeInstanceOf(Set);
    expect(result.current.existingCreatorIds.size).toBe(2);
    expect(result.current.existingCreatorIds.has(CREATOR_A)).toBe(true);
    expect(result.current.existingCreatorIds.has(CREATOR_B)).toBe(true);
  });

  it("existingCreatorIds is empty when no creators are attached to the campaign", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([]);

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.existingCreatorIds.size).toBe(0);
  });

  it("existingCreatorIds includes ids whose creator profile is missing (soft-deleted)", async () => {
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([
      makeCC(CREATOR_A),
      makeCC(CREATOR_B),
    ]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [makeCreator(CREATOR_A, "Aлексей")],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.existingCreatorIds.has(CREATOR_A)).toBe(true);
    expect(result.current.existingCreatorIds.has(CREATOR_B)).toBe(true);
  });

  it("chunks profile fetch into 200-id batches when a campaign has 200+ members (backend caps `ids` at 200)", async () => {
    const ids = Array.from(
      { length: 250 },
      (_, i) =>
        `${i.toString(16).padStart(8, "0")}-aaaa-aaaa-aaaa-aaaaaaaaaaaa`,
    );
    const ccs = ids.map((id) => makeCC(id));
    const profiles = ids.map((id, i) => makeCreator(id, `Last${i}`));

    vi.mocked(listCampaignCreators).mockResolvedValueOnce(ccs);
    vi.mocked(listCreators).mockImplementation(async (input) => ({
      data: {
        items: profiles.filter((p) => input.ids?.includes(p.id)),
        total: input.ids?.length ?? 0,
        page: 1,
        perPage: input.perPage,
      },
    }));

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.rows).toHaveLength(250);
    });

    // 250 / 200 → 2 chunks (200 + 50). Without chunking the backend would
    // return 422 «не более 200 элементов» and the section would land in
    // ErrorState — this is the bug the manual subagent caught against PR #86.
    expect(listCreators).toHaveBeenCalledTimes(2);
    const calls = vi.mocked(listCreators).mock.calls;
    const firstChunkSize = calls[0]?.[0].ids?.length ?? 0;
    const secondChunkSize = calls[1]?.[0].ids?.length ?? 0;
    expect([firstChunkSize, secondChunkSize].sort((a, b) => a - b)).toEqual([
      50, 200,
    ]);
  });

  it("refetch retries A3 and listCreators", async () => {
    const ccA = makeCC(CREATOR_A);
    const creatorA = makeCreator(CREATOR_A, "Aлексей");

    vi.mocked(listCampaignCreators).mockResolvedValue([ccA]);
    vi.mocked(listCreators).mockResolvedValue({
      data: { items: [creatorA], total: 1, page: 1, perPage: 200 },
    });

    const { result } = renderHook(() => useCampaignCreators(CAMPAIGN_ID), {
      wrapper: wrap(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.rows).toHaveLength(1);
    });
    expect(listCampaignCreators).toHaveBeenCalledTimes(1);
    expect(listCreators).toHaveBeenCalledTimes(1);

    result.current.refetch();

    await waitFor(() => {
      expect(listCampaignCreators).toHaveBeenCalledTimes(2);
      expect(listCreators).toHaveBeenCalledTimes(2);
    });
  });
});
