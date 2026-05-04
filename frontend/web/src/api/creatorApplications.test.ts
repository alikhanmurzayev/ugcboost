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
import {
  listCreatorApplications,
  getCreatorApplication,
  getCreatorApplicationsCounts,
  verifyApplicationSocialManually,
} from "./creatorApplications";

const mockedPost = vi.mocked(client.POST);
const mockedGet = vi.mocked(client.GET);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("listCreatorApplications", () => {
  it("calls POST /creators/applications/list with body and returns data", async () => {
    mockedPost.mockResolvedValueOnce({
      data: {
        data: { items: [], total: 0, page: 1, perPage: 50 },
      },
      response: { status: 200 } as Response,
    });

    const input = {
      statuses: ["verification" as const],
      sort: "created_at" as const,
      order: "desc" as const,
      page: 1,
      perPage: 50,
      search: "Иван",
      cities: ["ALA"],
      dateFrom: "2026-04-01T00:00:00.000Z",
    };

    const result = await listCreatorApplications(input);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith("/creators/applications/list", {
      body: input,
    });
    expect(result).toEqual({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });
  });

  it("throws ApiError with code from error body on failure", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(
      listCreatorApplications({
        statuses: ["verification"],
        sort: "created_at",
        order: "desc",
        page: 1,
        perPage: 50,
      }),
    ).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      listCreatorApplications({
        statuses: ["verification"],
        sort: "created_at",
        order: "desc",
        page: 1,
        perPage: 50,
      }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});

describe("getCreatorApplication", () => {
  it("calls GET /creators/applications/{id} with path param", async () => {
    mockedGet.mockResolvedValueOnce({
      data: { data: { id: "uuid-1" } },
      response: { status: 200 } as Response,
    });

    const result = await getCreatorApplication("uuid-1");

    expect(mockedGet).toHaveBeenCalledWith("/creators/applications/{id}", {
      params: { path: { id: "uuid-1" } },
    });
    expect(result).toEqual({ data: { id: "uuid-1" } });
  });

  it("throws ApiError on 404", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "NOT_FOUND", message: "missing" } },
      response: { status: 404 } as Response,
    });

    await expect(getCreatorApplication("missing")).rejects.toMatchObject({
      status: 404,
      code: "NOT_FOUND",
    });
  });
});

describe("getCreatorApplicationsCounts", () => {
  it("calls GET /creators/applications/counts and returns sparse items", async () => {
    mockedGet.mockResolvedValueOnce({
      data: {
        data: {
          items: [{ status: "verification", count: 7 }],
        },
      },
      response: { status: 200 } as Response,
    });

    const result = await getCreatorApplicationsCounts();

    expect(mockedGet).toHaveBeenCalledWith("/creators/applications/counts");
    expect(result).toEqual({
      data: { items: [{ status: "verification", count: 7 }] },
    });
  });

  it("throws ApiError on 403 for non-admin", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(getCreatorApplicationsCounts()).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
    });
  });
});

describe("verifyApplicationSocialManually", () => {
  it("calls POST /creators/applications/{id}/socials/{socialId}/verify with empty body", async () => {
    mockedPost.mockResolvedValueOnce({
      data: { data: {} },
      response: { status: 200 } as Response,
    });

    await verifyApplicationSocialManually("app-1", "soc-2");

    expect(mockedPost).toHaveBeenCalledWith(
      "/creators/applications/{id}/socials/{socialId}/verify",
      {
        params: { path: { id: "app-1", socialId: "soc-2" } },
        body: {},
      },
    );
  });

  it("throws ApiError on 422 TELEGRAM_NOT_LINKED", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED" } },
      response: { status: 422 } as Response,
    });

    await expect(
      verifyApplicationSocialManually("app-1", "soc-2"),
    ).rejects.toMatchObject({
      status: 422,
      code: "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED",
    });
  });

  it("throws ApiError on 409 SOCIAL_ALREADY_VERIFIED", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED" } },
      response: { status: 409 } as Response,
    });

    await expect(
      verifyApplicationSocialManually("app-1", "soc-2"),
    ).rejects.toMatchObject({
      status: 409,
      code: "CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedPost.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(
      verifyApplicationSocialManually("app-1", "soc-2"),
    ).rejects.toBeInstanceOf(ApiError);
  });
});
