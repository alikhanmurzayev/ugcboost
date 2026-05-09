import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { CAMPAIGN_CREATOR_STATUS } from "../../shared/constants/campaignCreatorStatus";
import { useAgreeDecision, useDeclineDecision } from "./useDecision";

vi.mock("../../api/client", () => ({
  apiClient: {
    POST: vi.fn(),
  },
}));

import { apiClient } from "../../api/client";

const mockedPost = vi.mocked(apiClient.POST);

function wrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useAgreeDecision", () => {
  beforeEach(() => {
    mockedPost.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("calls /agree path with the secret token and returns the typed result", async () => {
    mockedPost.mockResolvedValue({
      data: {
        status: CAMPAIGN_CREATOR_STATUS.AGREED,
        alreadyDecided: false,
      },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_secret_token_xx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockedPost).toHaveBeenCalledWith(
      "/tma/campaigns/{secretToken}/agree",
      { params: { path: { secretToken: "tok_secret_token_xx" } } },
    );
    expect(result.current.data).toEqual({
      status: CAMPAIGN_CREATOR_STATUS.AGREED,
      alreadyDecided: false,
    });
  });

  it("propagates already_decided=true through to the data shape", async () => {
    mockedPost.mockResolvedValue({
      data: {
        status: CAMPAIGN_CREATOR_STATUS.AGREED,
        alreadyDecided: true,
      },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({
      status: CAMPAIGN_CREATOR_STATUS.AGREED,
      alreadyDecided: true,
    });
  });

  it("surfaces backend ErrorResponse code on failure", async () => {
    mockedPost.mockResolvedValue({
      error: {
        error: { code: "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE", message: "need reinvite" },
      },
      response: { status: 422 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toEqual({
      status: 422,
      code: "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE",
      message: "need reinvite",
    });
  });

  it("rejects with NETWORK_ERROR when apiClient throws", async () => {
    mockedPost.mockRejectedValue(new TypeError("Failed to fetch"));
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toEqual({
      status: 0,
      code: "NETWORK_ERROR",
      message: "",
    });
  });

  it("rejects with INTERNAL_ERROR when payload shape is unexpected", async () => {
    mockedPost.mockResolvedValue({
      data: { status: "unknown", alreadyDecided: "no" },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toEqual({
      status: 200,
      code: "INTERNAL_ERROR",
      message: "",
    });
  });

  it("rejects without calling the API when secretToken is undefined", async () => {
    const { result } = renderHook(() => useAgreeDecision(undefined), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(mockedPost).not.toHaveBeenCalled();
    expect(result.current.error).toEqual({
      status: 0,
      code: "INTERNAL_ERROR",
      message: "secretToken missing",
    });
  });
});

describe("useDeclineDecision", () => {
  beforeEach(() => {
    mockedPost.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("calls /decline path with the secret token", async () => {
    mockedPost.mockResolvedValue({
      data: {
        status: CAMPAIGN_CREATOR_STATUS.DECLINED,
        alreadyDecided: false,
      },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(() => useDeclineDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockedPost).toHaveBeenCalledWith(
      "/tma/campaigns/{secretToken}/decline",
      { params: { path: { secretToken: "tok_xxxxxxxxxxxxxxxx" } } },
    );
    expect(result.current.data).toEqual({
      status: CAMPAIGN_CREATOR_STATUS.DECLINED,
      alreadyDecided: false,
    });
  });

  it("rejects with NETWORK_ERROR when apiClient throws on decline", async () => {
    mockedPost.mockRejectedValue(new TypeError("Failed to fetch"));
    const { result } = renderHook(() => useDeclineDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.code).toBe("NETWORK_ERROR");
  });
});
