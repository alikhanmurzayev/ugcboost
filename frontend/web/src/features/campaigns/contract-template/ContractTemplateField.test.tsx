import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ContractTemplateField from "./ContractTemplateField";
import { ApiError } from "@/api/client";

vi.mock("@/api/campaigns", () => ({
  uploadCampaignContractTemplate: vi.fn(),
  downloadCampaignContractTemplate: vi.fn(),
}));

import {
  uploadCampaignContractTemplate,
  downloadCampaignContractTemplate,
} from "@/api/campaigns";

function renderField(
  props: Partial<React.ComponentProps<typeof ContractTemplateField>> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const merged = {
    campaignId: "c-1",
    campaignName: "Promo Spring",
    hasTemplate: false,
    ...props,
  };
  return render(
    <QueryClientProvider client={queryClient}>
      <ContractTemplateField {...merged} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ContractTemplateField", () => {
  it("hasTemplate=false: renders upload button only", () => {
    renderField({ hasTemplate: false });
    expect(
      screen.getByTestId("contract-template-upload-button"),
    ).toHaveTextContent("Загрузить шаблон");
    expect(
      screen.queryByTestId("contract-template-replace-button"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("contract-template-download-button"),
    ).not.toBeInTheDocument();
  });

  it("hasTemplate=true: renders replace + download buttons", () => {
    renderField({ hasTemplate: true });
    expect(
      screen.getByTestId("contract-template-replace-button"),
    ).toHaveTextContent("Заменить шаблон");
    expect(
      screen.getByTestId("contract-template-download-button"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("contract-template-upload-button"),
    ).not.toBeInTheDocument();
  });

  it("successful upload renders preview block with placeholders", async () => {
    vi.mocked(uploadCampaignContractTemplate).mockResolvedValue({
      data: {
        hash: "deadbeef",
        placeholders: ["CreatorFIO", "CreatorIIN", "IssuedDate"],
      },
    });
    const user = userEvent.setup();
    renderField({ hasTemplate: false });

    const fileInput = screen.getByTestId(
      "contract-template-input",
    ) as HTMLInputElement;
    const file = new File(["%PDF-1.4 fake"], "contract.pdf", {
      type: "application/pdf",
    });
    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(
        screen.getByTestId("contract-template-placeholders"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByTestId("contract-template-placeholder-CreatorFIO"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("contract-template-placeholder-CreatorIIN"),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("contract-template-placeholder-IssuedDate"),
    ).toBeInTheDocument();
  });

  it("upload error renders backend message inline", async () => {
    vi.mocked(uploadCampaignContractTemplate).mockRejectedValue(
      new ApiError(
        422,
        "CONTRACT_MISSING_PLACEHOLDER",
        "В шаблоне не найдены обязательные плейсхолдеры: {{CreatorIIN}}.",
        { missing: ["CreatorIIN"] },
      ),
    );
    const user = userEvent.setup();
    renderField({ hasTemplate: false });

    const fileInput = screen.getByTestId(
      "contract-template-input",
    ) as HTMLInputElement;
    const file = new File(["%PDF-1.4 fake"], "contract.pdf", {
      type: "application/pdf",
    });
    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(screen.getByTestId("contract-template-error")).toHaveTextContent(
        "В шаблоне не найдены обязательные плейсхолдеры: {{CreatorIIN}}.",
      );
    });
    expect(
      screen.queryByTestId("contract-template-placeholders"),
    ).not.toBeInTheDocument();
  });

  it("network error falls back to localized message", async () => {
    vi.mocked(uploadCampaignContractTemplate).mockRejectedValue(
      new Error("network down"),
    );
    const user = userEvent.setup();
    renderField({ hasTemplate: false });
    const fileInput = screen.getByTestId(
      "contract-template-input",
    ) as HTMLInputElement;
    const file = new File(["%PDF-1.4 fake"], "contract.pdf", {
      type: "application/pdf",
    });
    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(screen.getByTestId("contract-template-error")).toHaveTextContent(
        "Не удалось загрузить шаблон. Попробуйте снова.",
      );
    });
  });

  it("download triggers anchor click with blob URL", async () => {
    const blob = new Blob(["%PDF stored"], { type: "application/pdf" });
    vi.mocked(downloadCampaignContractTemplate).mockResolvedValue(blob);

    const createObjectURL = vi.fn(() => "blob:fake");
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: createObjectURL,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revokeObjectURL,
    });

    const user = userEvent.setup();
    renderField({ hasTemplate: true });

    const downloadBtn = screen.getByTestId(
      "contract-template-download-button",
    );
    await user.click(downloadBtn);

    await waitFor(() => {
      expect(downloadCampaignContractTemplate).toHaveBeenCalledWith("c-1");
    });
    expect(createObjectURL).toHaveBeenCalledWith(blob);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:fake");
  });

  it("disabled state turns upload off", () => {
    renderField({ hasTemplate: false, disabled: true });
    expect(
      screen.getByTestId("contract-template-upload-button"),
    ).toBeDisabled();
  });
});
