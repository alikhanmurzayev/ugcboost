import { beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  triggerDownloadContractTemplate,
  useUploadContractTemplate,
} from "./useContractTemplate";

vi.mock("@/api/campaigns", () => ({
  uploadCampaignContractTemplate: vi.fn(),
  downloadCampaignContractTemplate: vi.fn(),
}));

import {
  uploadCampaignContractTemplate,
  downloadCampaignContractTemplate,
} from "@/api/campaigns";

function withQueryClient() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return { queryClient, wrapper };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useUploadContractTemplate", () => {
  it("calls api with file and resolves with result", async () => {
    vi.mocked(uploadCampaignContractTemplate).mockResolvedValue({
      data: { hash: "h", placeholders: ["CreatorFIO", "CreatorIIN", "IssuedDate"] },
    });
    const { wrapper } = withQueryClient();
    const { result } = renderHook(() => useUploadContractTemplate("c-1"), {
      wrapper,
    });

    const file = new File(["%PDF"], "x.pdf", { type: "application/pdf" });
    result.current.mutate(file);
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(uploadCampaignContractTemplate).toHaveBeenCalledWith("c-1", file);
    expect(result.current.data?.data.placeholders).toEqual([
      "CreatorFIO",
      "CreatorIIN",
      "IssuedDate",
    ]);
  });

  it("propagates rejected error", async () => {
    const err = new Error("boom");
    vi.mocked(uploadCampaignContractTemplate).mockRejectedValue(err);
    const { wrapper } = withQueryClient();
    const { result } = renderHook(() => useUploadContractTemplate("c-1"), {
      wrapper,
    });
    result.current.mutate(new File([], "x.pdf"));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBe(err);
  });
});

describe("triggerDownloadContractTemplate", () => {
  it("downloads blob and clicks anchor", async () => {
    const blob = new Blob(["%PDF"], { type: "application/pdf" });
    vi.mocked(downloadCampaignContractTemplate).mockResolvedValue(blob);

    const created = vi.fn(() => "blob:fake");
    const revoked = vi.fn();
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: created,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revoked,
    });

    await triggerDownloadContractTemplate("c-1", "promo.pdf");

    expect(downloadCampaignContractTemplate).toHaveBeenCalledWith("c-1");
    expect(created).toHaveBeenCalledWith(blob);
    expect(revoked).toHaveBeenCalledWith("blob:fake");
  });

  it("revokes URL even when click throws", async () => {
    const blob = new Blob(["%PDF"], { type: "application/pdf" });
    vi.mocked(downloadCampaignContractTemplate).mockResolvedValue(blob);

    const created = vi.fn(() => "blob:fake");
    const revoked = vi.fn();
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: created,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revoked,
    });

    // Patch document.createElement to return an anchor whose click throws.
    const originalCreate = document.createElement.bind(document);
    const spy = vi
      .spyOn(document, "createElement")
      .mockImplementation((tag: string) => {
        if (tag === "a") {
          const a = originalCreate(tag) as HTMLAnchorElement;
          a.click = () => {
            throw new Error("click failed");
          };
          return a;
        }
        return originalCreate(tag);
      });

    await expect(
      triggerDownloadContractTemplate("c-1", "promo.pdf"),
    ).rejects.toThrow("click failed");
    expect(revoked).toHaveBeenCalledWith("blob:fake");

    spy.mockRestore();
  });
});
