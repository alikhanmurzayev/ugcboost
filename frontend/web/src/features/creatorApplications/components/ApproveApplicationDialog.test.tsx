import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ApproveApplicationDialog from "./ApproveApplicationDialog";

vi.mock("@/api/creatorApplications", () => ({
  approveApplication: vi.fn(),
}));

vi.mock("@/api/campaigns", () => ({
  listCampaigns: vi.fn(),
}));

vi.mock("@/api/client", async () => {
  class ApiError extends Error {
    status: number;
    code: string;
    serverMessage?: string;
    constructor(status: number, code: string, serverMessage?: string) {
      super(code);
      this.status = status;
      this.code = code;
      this.serverMessage = serverMessage;
    }
  }
  return { ApiError };
});

import { approveApplication } from "@/api/creatorApplications";
import { listCampaigns } from "@/api/campaigns";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";

const ISO = "2026-05-07T12:00:00Z";

function campaignsFixture() {
  return {
    data: {
      items: [
        {
          id: "11111111-1111-1111-1111-111111111111",
          name: "Promo A",
          tmaUrl: "https://tma/a",
          isDeleted: false,
          createdAt: ISO,
          updatedAt: ISO,
        },
        {
          id: "22222222-2222-2222-2222-222222222222",
          name: "Promo B",
          tmaUrl: "https://tma/b",
          isDeleted: false,
          createdAt: ISO,
          updatedAt: ISO,
        },
      ],
      total: 2,
      page: 1,
      perPage: 100,
    },
  };
}

function setup() {
  vi.mocked(listCampaigns).mockResolvedValue(campaignsFixture());
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
  const onApiError = vi.fn();
  const onCloseDrawer = vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <ApproveApplicationDialog
        applicationId="app-1"
        onApiError={onApiError}
        onCloseDrawer={onCloseDrawer}
      />
    </QueryClientProvider>,
  );
  return { ...utils, queryClient, invalidateSpy, onApiError, onCloseDrawer };
}

// openAndArm clicks the trigger button and waits for the campaigns query to
// resolve so the submit button leaves its async-prereq disabled state. Tests
// that drive submit must await this — otherwise the submit click hits the
// guard inside handleSubmit and the mutation never starts.
async function openAndArm() {
  await userEvent.click(screen.getByTestId("approve-button"));
  await waitFor(() => {
    expect(screen.getByTestId("approve-confirm-submit")).not.toBeDisabled();
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ApproveApplicationDialog — trigger button", () => {
  it("renders trigger button labelled «Одобрить заявку», dialog initially hidden", () => {
    setup();
    expect(screen.getByTestId("approve-button")).toHaveTextContent("Одобрить заявку");
    expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
  });

  it("opens dialog on trigger click", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
  });
});

describe("ApproveApplicationDialog — dialog body", () => {
  it("shows fixed body referencing Telegram-bot notification", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    expect(
      screen.getByText(/Креатор получит уведомление в Telegram-боте/),
    ).toBeInTheDocument();
  });

  it("renders campaign multiselect with optional-label hint", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    expect(screen.getByText("Добавить в кампании (опционально)")).toBeInTheDocument();
    expect(screen.getByTestId("approve-campaigns-multiselect")).toBeInTheDocument();
  });
});

describe("ApproveApplicationDialog — campaigns query", () => {
  it("fetches campaigns only when dialog opens", async () => {
    setup();
    expect(listCampaigns).not.toHaveBeenCalled();
    await userEvent.click(screen.getByTestId("approve-button"));
    await waitFor(() => {
      expect(listCampaigns).toHaveBeenCalledWith({
        page: 1,
        perPage: 100,
        sort: "name",
        order: "asc",
        isDeleted: false,
      });
    });
  });

  it("disables submit while campaigns are loading", async () => {
    vi.mocked(listCampaigns).mockImplementationOnce(() => new Promise(() => {}));
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={queryClient}>
        <ApproveApplicationDialog
          applicationId="app-1"
          onApiError={vi.fn()}
          onCloseDrawer={vi.fn()}
        />
      </QueryClientProvider>,
    );
    await userEvent.click(screen.getByTestId("approve-button"));
    expect(screen.getByTestId("approve-confirm-submit")).toBeDisabled();
  });

  it("renders inline campaigns load-error when listCampaigns fails", async () => {
    vi.mocked(listCampaigns).mockRejectedValueOnce(new ApiError(500, "INTERNAL_ERROR"));
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={queryClient}>
        <ApproveApplicationDialog
          applicationId="app-1"
          onApiError={vi.fn()}
          onCloseDrawer={vi.fn()}
        />
      </QueryClientProvider>,
    );
    await userEvent.click(screen.getByTestId("approve-button"));
    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-campaigns-error")).toBeInTheDocument();
    });
  });
});

describe("ApproveApplicationDialog — cancel", () => {
  it("closes dialog when cancel clicked", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-cancel"));
    expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
  });

  it("closes dialog when backdrop clicked", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-backdrop"));
    expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
  });
});

describe("ApproveApplicationDialog — submit success", () => {
  it("submit без выбора кампаний → approveApplication(id, []), invalidates creator-applications", async () => {
    vi.mocked(approveApplication).mockResolvedValueOnce({
      data: { creatorId: "creator-1" },
    });
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await waitFor(() => {
      expect(screen.getByTestId("approve-confirm-submit")).not.toBeDisabled();
    });
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(approveApplication).toHaveBeenCalledWith("app-1", []);
    });
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["creator-applications"],
      });
    });
    await waitFor(() => {
      expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
    });
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("submit с выбранными кампаниями → forwards ids and invalidates per-campaign keys", async () => {
    vi.mocked(approveApplication).mockResolvedValueOnce({
      data: { creatorId: "creator-1" },
    });
    const { invalidateSpy } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-campaigns-multiselect"));
    await userEvent.click(
      screen.getByTestId("approve-campaigns-multiselect-option-11111111-1111-1111-1111-111111111111"),
    );
    await userEvent.click(
      screen.getByTestId("approve-campaigns-multiselect-option-22222222-2222-2222-2222-222222222222"),
    );
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(approveApplication).toHaveBeenCalledWith("app-1", [
        "11111111-1111-1111-1111-111111111111",
        "22222222-2222-2222-2222-222222222222",
      ]);
    });
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["campaigns", "detail", "11111111-1111-1111-1111-111111111111"],
      });
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["campaignCreators", "list", "11111111-1111-1111-1111-111111111111"],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["campaigns", "detail", "22222222-2222-2222-2222-222222222222"],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["campaignCreators", "list", "22222222-2222-2222-2222-222222222222"],
    });
  });
});

describe("ApproveApplicationDialog — 4xx error handling", () => {
  it("on 422 NOT_APPROVABLE → onApiError(localized) + invalidate + close dialog, drawer stays", async () => {
    const code = "CREATOR_APPLICATION_NOT_APPROVABLE";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["creator-applications"],
    });
    expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD → inline error, dialog stays open, no invalidate", async () => {
    const code = "CAMPAIGN_NOT_AVAILABLE_FOR_ADD";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-error")).toHaveTextContent(
        getErrorMessage(code),
      );
    });
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("on 422 CAMPAIGN_ADD_AFTER_APPROVE_FAILED → inline error with serverMessage + invalidate creators/campaigns", async () => {
    const code = "CAMPAIGN_ADD_AFTER_APPROVE_FAILED";
    const serverMessage = "Не удалось добавить креатора в кампанию camp-x. Креатор уже создан — добавьте его вручную через страницу кампании.";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code, serverMessage));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-error")).toHaveTextContent(serverMessage);
    });
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["creator-applications"] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["creators"] });
    expect(onCloseDrawer).not.toHaveBeenCalled();
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("on 422 CAMPAIGN_IDS_TOO_MANY → inline error, dialog stays open", async () => {
    const code = "CAMPAIGN_IDS_TOO_MANY";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { onCloseDrawer } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-error")).toBeInTheDocument();
    });
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 404 NOT_FOUND → onApiError + close dialog AND drawer", async () => {
    const code = "NOT_FOUND";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(404, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalled();
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
  });
});

describe("ApproveApplicationDialog — network/5xx error", () => {
  it("on 500 INTERNAL_ERROR → fallback retry-error, dialog stays, no invalidate, no drawer close", async () => {
    vi.mocked(approveApplication).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-error")).toHaveTextContent(
        "Не удалось одобрить, попробуйте ещё раз",
      );
    });
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("on non-ApiError (network/throw) → inline error, dialog stays", async () => {
    vi.mocked(approveApplication).mockRejectedValueOnce(new Error("network"));
    const { invalidateSpy, onCloseDrawer } = setup();

    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("approve-dialog-error")).toBeInTheDocument();
    });
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });
});

describe("ApproveApplicationDialog — Escape key", () => {
  it("closes dialog on Escape when not pending", async () => {
    setup();
    await userEvent.click(screen.getByTestId("approve-button"));
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByTestId("approve-confirm-dialog")).not.toBeInTheDocument();
  });

  it("ignores Escape while pending", async () => {
    vi.mocked(approveApplication).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();
    await openAndArm();
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));
    await waitFor(() => {
      expect(screen.getByTestId("approve-confirm-submit")).toBeDisabled();
    });
    await userEvent.keyboard("{Escape}");
    expect(screen.getByTestId("approve-confirm-dialog")).toBeInTheDocument();
  });
});

describe("ApproveApplicationDialog — pending state & double-submit guard", () => {
  it("disables submit, cancel and backdrop while pending; submit text changes", async () => {
    vi.mocked(approveApplication).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    const submit = screen.getByTestId("approve-confirm-submit");
    await userEvent.click(submit);

    await waitFor(() => {
      expect(submit).toBeDisabled();
    });
    expect(screen.getByTestId("approve-confirm-cancel")).toBeDisabled();
    expect(screen.getByTestId("approve-confirm-backdrop")).toBeDisabled();
    expect(submit).toHaveTextContent("Одобрение...");
  });

  it("ignores subsequent submit clicks while pending (double-submit guard)", async () => {
    vi.mocked(approveApplication).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    const submit = screen.getByTestId("approve-confirm-submit");
    await userEvent.click(submit);
    await userEvent.click(submit);
    await userEvent.click(submit);

    await waitFor(() => {
      expect(approveApplication).toHaveBeenCalledTimes(1);
    });
  });
});
