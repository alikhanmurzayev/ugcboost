import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("./client", () => {
  class ApiError extends Error {
    status: number;
    code: string;
    serverMessage?: string;
    details?: unknown;
    constructor(
      status: number,
      code: string,
      serverMessage?: string,
      details?: unknown,
    ) {
      super(code);
      this.status = status;
      this.code = code;
      this.serverMessage = serverMessage;
      this.details = details;
    }
  }
  return {
    default: {
      POST: vi.fn(),
      GET: vi.fn(),
      DELETE: vi.fn(),
    },
    ApiError,
  };
});

import client from "./client";
import {
  listCampaignCreators,
  addCampaignCreators,
  removeCampaignCreator,
  notifyCampaignCreators,
  remindCampaignCreatorsInvitation,
  remindCampaignCreatorsSigning,
} from "./campaignCreators";

const mockedGet = vi.mocked(client.GET);
const mockedPost = vi.mocked(client.POST);
const mockedDelete = vi.mocked(client.DELETE);

const CAMPAIGN_ID = "11111111-1111-1111-1111-111111111111";
const CREATOR_A = "22222222-2222-2222-2222-222222222222";
const CREATOR_B = "33333333-3333-3333-3333-333333333333";

const FIXTURE_CC = {
  id: "cc-1",
  campaignId: CAMPAIGN_ID,
  creatorId: CREATOR_A,
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

  it("falls back to status=500 + code=INTERNAL_ERROR on malformed error body", async () => {
    mockedGet.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(listCampaignCreators(CAMPAIGN_ID)).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });

  it("falls back to status=500 + code=INTERNAL_ERROR when error.code is non-string", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: 42 } },
      response: { status: 500 } as Response,
    });

    await expect(listCampaignCreators(CAMPAIGN_ID)).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});

describe("addCampaignCreators", () => {
  it("calls POST /campaigns/{id}/creators with creatorIds and unwraps items", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { items: [FIXTURE_CC] } },
      response: { status: 201 } as Response,
    });

    const result = await addCampaignCreators(CAMPAIGN_ID, [CREATOR_A]);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith("/campaigns/{id}/creators", {
      params: { path: { id: CAMPAIGN_ID } },
      body: { creatorIds: [CREATOR_A] },
    });
    expect(result).toEqual([FIXTURE_CC]);
  });

  it("supports batch with multiple creator ids", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { items: [FIXTURE_CC, { ...FIXTURE_CC, id: "cc-2", creatorId: CREATOR_B }] } },
      response: { status: 201 } as Response,
    });

    const result = await addCampaignCreators(CAMPAIGN_ID, [CREATOR_A, CREATOR_B]);

    expect(mockedPost).toHaveBeenCalledWith("/campaigns/{id}/creators", {
      params: { path: { id: CAMPAIGN_ID } },
      body: { creatorIds: [CREATOR_A, CREATOR_B] },
    });
    expect(result).toHaveLength(2);
  });

  it("throws ApiError with code on 422 CREATOR_ALREADY_IN_CAMPAIGN", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CREATOR_ALREADY_IN_CAMPAIGN", message: "race" } },
      response: { status: 422 } as Response,
    });

    await expect(
      addCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 422,
      code: "CREATOR_ALREADY_IN_CAMPAIGN",
    });
  });

  it("throws ApiError with code on 404 CAMPAIGN_NOT_FOUND", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_NOT_FOUND", message: "soft-deleted" } },
      response: { status: 404 } as Response,
    });

    await expect(
      addCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 404,
      code: "CAMPAIGN_NOT_FOUND",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed 5xx body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      addCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});

describe("removeCampaignCreator", () => {
  it("calls DELETE /campaigns/{id}/creators/{creatorId} with both path params", async () => {
    mockedDelete.mockResolvedValueOnce({
      response: { status: 204 } as Response,
    });

    await removeCampaignCreator(CAMPAIGN_ID, CREATOR_A);

    expect(mockedDelete).toHaveBeenCalledTimes(1);
    expect(mockedDelete).toHaveBeenCalledWith(
      "/campaigns/{id}/creators/{creatorId}",
      { params: { path: { id: CAMPAIGN_ID, creatorId: CREATOR_A } } },
    );
  });

  it("returns void on 204 with no body", async () => {
    mockedDelete.mockResolvedValueOnce({
      response: { status: 204 } as Response,
    });

    const result = await removeCampaignCreator(CAMPAIGN_ID, CREATOR_A);

    expect(result).toBeUndefined();
  });

  it("throws ApiError with code on 404 race", async () => {
    mockedDelete.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_CREATOR_NOT_FOUND", message: "race" } },
      response: { status: 404 } as Response,
    });

    await expect(
      removeCampaignCreator(CAMPAIGN_ID, CREATOR_A),
    ).rejects.toMatchObject({
      status: 404,
      code: "CAMPAIGN_CREATOR_NOT_FOUND",
    });
  });

  it("throws ApiError with code on 422 CREATOR_REMOVE_AFTER_AGREED", async () => {
    mockedDelete.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_CREATOR_REMOVE_AFTER_AGREED", message: "agreed" } },
      response: { status: 422 } as Response,
    });

    await expect(
      removeCampaignCreator(CAMPAIGN_ID, CREATOR_A),
    ).rejects.toMatchObject({
      status: 422,
      code: "CAMPAIGN_CREATOR_REMOVE_AFTER_AGREED",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed 5xx body", async () => {
    mockedDelete.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      removeCampaignCreator(CAMPAIGN_ID, CREATOR_A),
    ).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});

describe("notifyCampaignCreators", () => {
  it("calls POST /campaigns/{id}/notify with creatorIds and returns the full CampaignNotifyResult envelope", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { undelivered: [] } },
      response: { status: 200 } as Response,
    });

    const result = await notifyCampaignCreators(CAMPAIGN_ID, [CREATOR_A]);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith("/campaigns/{id}/notify", {
      params: { path: { id: CAMPAIGN_ID } },
      body: { creatorIds: [CREATOR_A] },
    });
    expect(result).toEqual({ data: { undelivered: [] } });
  });

  it("returns partial-undelivered list when delivery fails for some creators", async () => {
    mockedPost.mockResolvedValueOnce({
      data: {
        data: {
          undelivered: [{ creatorId: CREATOR_A, reason: "bot_blocked" }],
        },
      },
      response: { status: 200 } as Response,
    });

    const result = await notifyCampaignCreators(CAMPAIGN_ID, [CREATOR_A, CREATOR_B]);

    expect(result.data.undelivered).toEqual([
      { creatorId: CREATOR_A, reason: "bot_blocked" },
    ]);
  });

  it("throws ApiError with code+details on 422 CAMPAIGN_CREATOR_BATCH_INVALID", async () => {
    const details = [
      { creatorId: CREATOR_A, reason: "wrong_status", currentStatus: "invited" },
    ];
    mockedPost.mockResolvedValueOnce({
      error: {
        error: {
          code: "CAMPAIGN_CREATOR_BATCH_INVALID",
          message: "batch invalid",
          details,
        },
      },
      response: { status: 422 } as Response,
    });

    await expect(
      notifyCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 422,
      code: "CAMPAIGN_CREATOR_BATCH_INVALID",
      details,
    });
  });

  it("throws ApiError on 404 CAMPAIGN_NOT_FOUND (soft-deleted campaign)", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_NOT_FOUND", message: "missing" } },
      response: { status: 404 } as Response,
    });

    await expect(
      notifyCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 404,
      code: "CAMPAIGN_NOT_FOUND",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed 5xx body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      notifyCampaignCreators(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});

describe("remindCampaignCreatorsInvitation", () => {
  it("calls POST /campaigns/{id}/remind-invitation with creatorIds and returns the envelope", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { undelivered: [] } },
      response: { status: 200 } as Response,
    });

    const result = await remindCampaignCreatorsInvitation(CAMPAIGN_ID, [CREATOR_A]);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith(
      "/campaigns/{id}/remind-invitation",
      {
        params: { path: { id: CAMPAIGN_ID } },
        body: { creatorIds: [CREATOR_A] },
      },
    );
    expect(result).toEqual({ data: { undelivered: [] } });
  });

  it("throws ApiError with code+details on 422 CAMPAIGN_CREATOR_BATCH_INVALID", async () => {
    const details = [
      { creatorId: CREATOR_A, reason: "wrong_status", currentStatus: "planned" },
    ];
    mockedPost.mockResolvedValueOnce({
      error: {
        error: {
          code: "CAMPAIGN_CREATOR_BATCH_INVALID",
          message: "batch invalid",
          details,
        },
      },
      response: { status: 422 } as Response,
    });

    await expect(
      remindCampaignCreatorsInvitation(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 422,
      code: "CAMPAIGN_CREATOR_BATCH_INVALID",
      details,
    });
  });

  it("falls back to INTERNAL_ERROR on malformed 5xx body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      remindCampaignCreatorsInvitation(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});

describe("remindCampaignCreatorsSigning", () => {
  it("calls POST /campaigns/{id}/remind-signing with creatorIds and returns the envelope", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { undelivered: [] } },
      response: { status: 200 } as Response,
    });

    const result = await remindCampaignCreatorsSigning(CAMPAIGN_ID, [CREATOR_A]);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith(
      "/campaigns/{id}/remind-signing",
      {
        params: { path: { id: CAMPAIGN_ID } },
        body: { creatorIds: [CREATOR_A] },
      },
    );
    expect(result).toEqual({ data: { undelivered: [] } });
  });

  it("throws ApiError with code+details on 422 CAMPAIGN_CREATOR_BATCH_INVALID", async () => {
    const details = [
      { creatorId: CREATOR_A, reason: "wrong_status", currentStatus: "agreed" },
    ];
    mockedPost.mockResolvedValueOnce({
      error: {
        error: {
          code: "CAMPAIGN_CREATOR_BATCH_INVALID",
          message: "batch invalid",
          details,
        },
      },
      response: { status: 422 } as Response,
    });

    await expect(
      remindCampaignCreatorsSigning(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 422,
      code: "CAMPAIGN_CREATOR_BATCH_INVALID",
      details,
    });
  });

  it("falls back to INTERNAL_ERROR on malformed 5xx body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      remindCampaignCreatorsSigning(CAMPAIGN_ID, [CREATOR_A]),
    ).rejects.toMatchObject({
      status: 500,
      code: "INTERNAL_ERROR",
    });
  });
});
