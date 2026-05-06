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
import { listCreators, getCreator } from "./creators";

const mockedPost = vi.mocked(client.POST);
const mockedGet = vi.mocked(client.GET);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("listCreators", () => {
  it("calls POST /creators/list with body and returns data", async () => {
    mockedPost.mockResolvedValueOnce({
      data: {
        data: { items: [], total: 0, page: 1, perPage: 50 },
      },
      response: { status: 200 } as Response,
    });

    const input = {
      page: 1,
      perPage: 50,
      sort: "full_name" as const,
      order: "asc" as const,
      search: "Иван",
      cities: ["ALA"],
      categories: ["fashion"],
      ageFrom: 18,
      ageTo: 35,
      dateFrom: "2026-04-01T00:00:00.000Z",
      dateTo: "2026-05-01T23:59:59.999Z",
    };

    const result = await listCreators(input);

    expect(mockedPost).toHaveBeenCalledTimes(1);
    expect(mockedPost).toHaveBeenCalledWith("/creators/list", { body: input });
    expect(result).toEqual({
      data: { items: [], total: 0, page: 1, perPage: 50 },
    });
  });

  it("throws ApiError with code from error body on 403", async () => {
    mockedPost.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(
      listCreators({
        page: 1,
        perPage: 50,
        sort: "full_name",
        order: "asc",
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
      listCreators({
        page: 1,
        perPage: 50,
        sort: "full_name",
        order: "asc",
      }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});

describe("getCreator", () => {
  it("calls GET /creators/{id} with path param", async () => {
    mockedGet.mockResolvedValueOnce({
      data: { data: { id: "uuid-1" } },
      response: { status: 200 } as Response,
    });

    const result = await getCreator("uuid-1");

    expect(mockedGet).toHaveBeenCalledWith("/creators/{id}", {
      params: { path: { id: "uuid-1" } },
    });
    expect(result).toEqual({ data: { id: "uuid-1" } });
  });

  it("throws ApiError on 404", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "CREATOR_NOT_FOUND", message: "missing" } },
      response: { status: 404 } as Response,
    });

    await expect(getCreator("missing")).rejects.toMatchObject({
      status: 404,
      code: "CREATOR_NOT_FOUND",
    });
  });

  it("throws ApiError on 403", async () => {
    mockedGet.mockResolvedValueOnce({
      error: { error: { code: "FORBIDDEN", message: "no" } },
      response: { status: 403 } as Response,
    });

    await expect(getCreator("any")).rejects.toMatchObject({
      status: 403,
      code: "FORBIDDEN",
    });
  });

  it("falls back to INTERNAL_ERROR on malformed error body", async () => {
    mockedGet.mockResolvedValueOnce({
      error: {},
      response: { status: 500 } as Response,
    });

    await expect(getCreator("uuid-1")).rejects.toBeInstanceOf(ApiError);
  });
});
