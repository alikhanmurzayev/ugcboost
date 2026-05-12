import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { CAMPAIGN_CREATOR_STATUS } from "../../shared/constants/campaignCreatorStatus";
import { useParticipationStatus } from "./useParticipationStatus";

const NDA_ACCEPTED = true;

vi.mock("../../api/client", () => ({
  apiClient: {
    GET: vi.fn(),
  },
}));

import { apiClient } from "../../api/client";

const mockedGet = vi.mocked(apiClient.GET);

function wrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useParticipationStatus", () => {
  beforeEach(() => {
    mockedGet.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("calls /participation with the secret token and returns the typed result", async () => {
    mockedGet.mockResolvedValue({
      data: { status: CAMPAIGN_CREATOR_STATUS.INVITED },
      response: { status: 200 } as Response,
    } as never);
    const { result } = renderHook(
      () => useParticipationStatus("tok_secret_token_xx", NDA_ACCEPTED),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockedGet).toHaveBeenCalledWith(
      "/tma/campaigns/{secretToken}/participation",
      { params: { path: { secretToken: "tok_secret_token_xx" } } },
    );
    expect(result.current.data).toEqual({
      status: CAMPAIGN_CREATOR_STATUS.INVITED,
    });
  });

  it("does not call the API when secretToken is undefined", async () => {
    const { result } = renderHook(
      () => useParticipationStatus(undefined, NDA_ACCEPTED),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"));
    expect(mockedGet).not.toHaveBeenCalled();
  });

  it("does not call the API when secretToken fails the format regex", async () => {
    const { result } = renderHook(
      () => useParticipationStatus("short", NDA_ACCEPTED),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"));
    expect(mockedGet).not.toHaveBeenCalled();
  });

  it("does not call the API before NDA is accepted (fingerprint-leak guard)", async () => {
    const { result } = renderHook(
      () => useParticipationStatus("tok_secret_token_xx", false),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"));
    expect(mockedGet).not.toHaveBeenCalled();
  });
});
