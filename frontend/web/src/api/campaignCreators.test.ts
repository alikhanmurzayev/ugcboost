import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("./client", () => {
  class ApiError extends Error {
    status: number;
    code: string;
    constructor(status: number, code: string) {
      super(code);
      this.status = status;
      this.code = code;
    }
  }
  return {
    default: {
      POST: vi.fn(),
      GET: vi.fn(),
    },
    ApiError,
  };
});

import client, { ApiError } from "./client";
import { listCampaignCreators } from "./campaignCreators";

const mockedGet = vi.mocked(client.GET);

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";

const FIXTURE_CC = {
  id: "cc-1",
  campaignId: CAMPAIGN_ID,
  creatorId: "22222222-2222-2222-2222-222222222222",
  status: "planned" as const,
  invitedAt: null,
  invitedCount: 0,
  remindedAt: null,
  remindedCount: 0,
  decidedAt: null,
  createdAt: "2026-05-07T12:00:00Z",
  updatedAt: "2026-05-07T12:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("listCampaignCreators", () => {
  it("calls GET /campaigns/{id}/creators with path param and unwraps items", async () => {
    mockedGet.mockResolvedValueOnce({
      data: { data: { items: [FIXTURE_CC] } },
      response: { status: 200 } as Response,
    });

    const result = await listCampaignCreators(CAMPAIGN_ID);

    expect(mockedGet).toHaveBeenCalledTimes(1);
    expect(mockedGet).toHaveBeenCalledWith("/campaigns/{id}/creators", {
      params: { path: { id: CAMPAIGN_ID } },
    });
    expect(result).toEqual([FIXTURE_CC]);
  });

  it("returns empty array when the campaign has no creators", async () => {
    mockedGet.mockResolvedValueOnce({
      data: { data: { items: [] } },
      response: { status: 200 } as Response,
    });

    const result = await listCampaignCreators(CAMPAIGN_ID);

    expect(result).toEqual([]);
  });

  it("throws ApiError with code from error body on 404", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_NOT_FOUND", message: "missing" } },
      response: { status: 404 } as Response,
    });

    await expect(listCampaignCreators("missing")).rejects.toMatchObject({
      status: 404,
      code: "CAMPAIGN_NOT_FOUND",
    });
  });

  it("throws ApiError with code from error body on 403", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(listCampaignCreators(CAMPAIGN_ID)).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedGet.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(listCampaignCreators(CAMPAIGN_ID)).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});
