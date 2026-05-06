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
      GET: vi.fn(),
      POST: vi.fn(),
    },
    ApiError,
  };
});

import client, { ApiError } from "./client";
import { listCampaigns, createCampaign } from "./campaigns";

const mockedGet = vi.mocked(client.GET);
const mockedPost = vi.mocked(client.POST);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("listCampaigns", () => {
  it("calls GET /campaigns with query params and returns data", async () => {
    mockedGet.mockResolvedValueOnce({
      data: {
        data: { items: [], total: 0, page: 1, perPage: 50 },
      },
      response: { status: 200 } as Response,
    });

    const input = {
      page: 1,
      perPage: 50,
      sort: "created_at" as const,
      order: "desc" as const,
      search: "promo",
      isDeleted: false,
    };

    const result = await listCampaigns(input);

    expect(mockedGet).toHaveBeenCalledTimes(1);
    expect(mockedGet).toHaveBeenCalledWith("/campaigns", {
      params: { query: input },
    });
    expect(result).toEqual({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });
  });

  it("forwards query without isDeleted (showDeleted=on case)", async () => {
    mockedGet.mockResolvedValueOnce({
      data: {
        data: { items: [], total: 0, page: 1, perPage: 50 },
      },
      response: { status: 200 } as Response,
    });

    const input = {
      page: 1,
      perPage: 50,
      sort: "name" as const,
      order: "asc" as const,
    };

    await listCampaigns(input);

    expect(mockedGet).toHaveBeenCalledWith("/campaigns", {
      params: { query: input },
    });
  });

  it("throws ApiError with code from error body on 403", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(
      listCampaigns({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      }),
    ).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
    });
  });

  it("throws ApiError on 422 with code from error body", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "VALIDATION_ERROR", message: "bad sort" } },
      response: { status: 422 } as Response,
    });

    await expect(
      listCampaigns({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      }),
    ).rejects.toMatchObject({
      status: 422,
      code: "VALIDATION_ERROR",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedGet.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      listCampaigns({
        page: 1,
        perPage: 50,
        sort: "created_at",
        order: "desc",
      }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});

describe("createCampaign", () => {
  it("calls POST /campaigns with body and returns data on 201", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: { id: "11111111-2222-3333-4444-555555555555" } },
      response: { status: 201 } as Response,
    });

    const input = {
      name: "Spring promo",
      tmaUrl: "https://t.me/foo/bar",
    };
    const result = await createCampaign(input);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith("/campaigns", { body: input });
    expect(result).toEqual({
      data: { id: "11111111-2222-3333-4444-555555555555" },
    });
  });

  it("throws ApiError on 409 with code from error body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CAMPAIGN_NAME_TAKEN", message: "taken" } },
      response: { status: 409 } as Response,
    });

    await expect(
      createCampaign({ name: "Existing", tmaUrl: "https://t.me/x" }),
    ).rejects.toMatchObject({
      status: 409,
      code: "CAMPAIGN_NAME_TAKEN",
    });
  });

  it("throws ApiError on 422 with code from error body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {
        error: { code: "CAMPAIGN_NAME_TOO_LONG", message: "too long" },
      },
      response: { status: 422 } as Response,
    });

    await expect(
      createCampaign({ name: "x".repeat(300), tmaUrl: "https://t.me/x" }),
    ).rejects.toMatchObject({
      status: 422,
      code: "CAMPAIGN_NAME_TOO_LONG",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      createCampaign({ name: "Any", tmaUrl: "https://t.me/x" }),
    ).rejects.toMatchObject({ status: 500, code: "INTERNAL_ERROR" });
  });
});
