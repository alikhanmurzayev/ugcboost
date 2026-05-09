import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@telegram-apps/sdk", () => ({
  retrieveRawInitData: vi.fn(),
}));

import { retrieveRawInitData } from "@telegram-apps/sdk";
import { tmaInitDataMiddleware } from "./middleware";

const mockedRetrieve = vi.mocked(retrieveRawInitData);

describe("tmaInitDataMiddleware", () => {
  beforeEach(() => {
    mockedRetrieve.mockReset();
  });

  it("attaches Authorization: tma <init> when initData is available", () => {
    mockedRetrieve.mockReturnValue("auth_date=1&hash=h");
    const req = new Request("http://x/agree");
    // Middleware mutates the request and may also return a Request-like value;
    // we only assert the side effect on req.headers below.
    void tmaInitDataMiddleware.onRequest?.({
      schemaPath: "/tma/campaigns/{secretToken}/agree",
      params: {} as never,
      request: req,
      options: {} as never,
      id: "x",
    } as never);
    expect(req.headers.get("Authorization")).toBe("tma auth_date=1&hash=h");
  });

  it("does not set Authorization when retrieveRawInitData throws", () => {
    mockedRetrieve.mockImplementation(() => {
      throw new Error("not in TMA");
    });
    const req = new Request("http://x/agree");
    void tmaInitDataMiddleware.onRequest?.({
      schemaPath: "/tma/campaigns/{secretToken}/agree",
      params: {} as never,
      request: req,
      options: {} as never,
      id: "x",
    } as never);
    expect(req.headers.get("Authorization")).toBeNull();
  });

  it("does not set Authorization when initData is empty", () => {
    mockedRetrieve.mockReturnValue("");
    const req = new Request("http://x/agree");
    void tmaInitDataMiddleware.onRequest?.({
      schemaPath: "/tma/campaigns/{secretToken}/agree",
      params: {} as never,
      request: req,
      options: {} as never,
      id: "x",
    } as never);
    expect(req.headers.get("Authorization")).toBeNull();
  });
});
