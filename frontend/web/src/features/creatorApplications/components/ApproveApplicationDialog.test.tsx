import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ApproveApplicationDialog from "./ApproveApplicationDialog";

vi.mock("@/api/creatorApplications", () => ({
  approveApplication: vi.fn(),
}));

vi.mock("@/api/client", async () => {
  class ApiError extends Error {
    status: number;
    code: string;
    constructor(status: number, code: string) {
      super(code);
      this.status = status;
      this.code = code;
    }
  }
  return { ApiError };
});

import { approveApplication } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";

function setup() {
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
  it("calls approve api, invalidates all keys, closes dialog and drawer on 200", async () => {
    vi.mocked(approveApplication).mockResolvedValueOnce({
      data: { creatorId: "creator-1" },
    });
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(approveApplication).toHaveBeenCalledWith("app-1");
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
});

describe("ApproveApplicationDialog — 4xx error handling", () => {
  it("on 422 NOT_APPROVABLE → onApiError(localized) + invalidate + close dialog, drawer stays", async () => {
    const code = "CREATOR_APPLICATION_NOT_APPROVABLE";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
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

  it("on 422 TELEGRAM_NOT_LINKED → onApiError + invalidate + close dialog, drawer stays", async () => {
    const code = "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 422 CREATOR_ALREADY_EXISTS → onApiError + close dialog", async () => {
    const code = "CREATOR_ALREADY_EXISTS";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { onApiError, onCloseDrawer } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 422 CREATOR_TELEGRAM_ALREADY_TAKEN → onApiError + close dialog", async () => {
    const code = "CREATOR_TELEGRAM_ALREADY_TAKEN";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { onApiError, onCloseDrawer } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 403 FORBIDDEN → onApiError + invalidate + close dialog", async () => {
    const code = "FORBIDDEN";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(403, code));
    const { onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 404 NOT_FOUND → onApiError + invalidate + close dialog AND drawer", async () => {
    const code = "NOT_FOUND";
    vi.mocked(approveApplication).mockRejectedValueOnce(new ApiError(404, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
    await userEvent.click(screen.getByTestId("approve-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalled();
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
  });
});

describe("ApproveApplicationDialog — network/5xx error", () => {
  it("on 500 INTERNAL_ERROR → inline error in dialog, dialog stays, no invalidate, no drawer close", async () => {
    vi.mocked(approveApplication).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("approve-button"));
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

    await userEvent.click(screen.getByTestId("approve-button"));
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
    await userEvent.click(screen.getByTestId("approve-button"));
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
