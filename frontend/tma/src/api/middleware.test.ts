import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@telegram-apps/sdk", () => ({
  retrieveRawInitData: vi.fn(),
}));

import { retrieveRawInitData } from "@telegram-apps/sdk";
import { tmaInitDataMiddleware } from "./middleware";

const mockedRetrieve = vi.mocked(retrieveRawInitData);

function runOnRequest(req: Request) {
  return tmaInitDataMiddleware.onRequest?.({
    schemaPath: "/tma/campaigns/{secretToken}/agree",
    params: {} as never,
    request: req,
    options: {} as never,
    id: "x",
  } as never);
}

describe("tmaInitDataMiddleware", () => {
  beforeEach(() => {
    mockedRetrieve.mockReset();
  });

  it("attaches Authorization: tma <init> when initData is available and returns the same request", async () => {
    mockedRetrieve.mockReturnValue("auth_date=1&hash=h");
    const req = new Request("http://x/agree");
    const result = await runOnRequest(req);
    expect(req.headers.get("Authorization")).toBe("tma auth_date=1&hash=h");
    expect(result).toBe(req);
  });

  it("does not set Authorization when retrieveRawInitData throws and returns the same request", async () => {
    mockedRetrieve.mockImplementation(() => {
      throw new Error("not in TMA");
    });
    const req = new Request("http://x/agree");
    const result = await runOnRequest(req);
    expect(req.headers.get("Authorization")).toBeNull();
    expect(result).toBe(req);
  });

  it("does not set Authorization when initData is empty and returns the same request", async () => {
    mockedRetrieve.mockReturnValue("");
    const req = new Request("http://x/agree");
    const result = await runOnRequest(req);
    expect(req.headers.get("Authorization")).toBeNull();
    expect(result).toBe(req);
  });
});
