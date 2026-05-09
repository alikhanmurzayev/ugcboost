import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

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
  beforeEach(() => mockedPost.mockReset());

  it("calls /agree path with the secret token and returns the typed result", async () => {
    mockedPost.mockResolvedValue({
      data: { status: "agreed", alreadyDecided: false },
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
      status: "agreed",
      alreadyDecided: false,
    });
  });

  it("propagates already_decided=true through to the data shape", async () => {
    mockedPost.mockResolvedValue({
      data: { status: "agreed", alreadyDecided: true },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.alreadyDecided).toBe(true);
  });

  it("surfaces backend ErrorResponse code on failure", async () => {
    mockedPost.mockResolvedValue({
      error: {
        error: { code: "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE", message: "..." },
      },
      response: { status: 422 } as Response,
    } as never);
    const { result } = renderHook(() => useAgreeDecision("tok_xxxxxxxxxxxxxxxx"), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.code).toBe(
      "CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE",
    );
    expect(result.current.error?.status).toBe(422);
  });

  it("rejects without calling the API when secretToken is undefined", async () => {
    const { result } = renderHook(() => useAgreeDecision(undefined), {
      wrapper: wrapper(),
    });
    result.current.mutate();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(mockedPost).not.toHaveBeenCalled();
  });
});

describe("useDeclineDecision", () => {
  beforeEach(() => mockedPost.mockReset());

  it("calls /decline path with the secret token", async () => {
    mockedPost.mockResolvedValue({
      data: { status: "declined", alreadyDecided: false },
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
  });
});
