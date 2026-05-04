import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import RejectApplicationDialog from "./RejectApplicationDialog";

vi.mock("@/api/creatorApplications", () => ({
  rejectApplication: vi.fn(),
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

import { rejectApplication } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";

interface SetupProps {
  hasTelegram?: boolean;
}

function setup(props: SetupProps = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
  const onApiError = vi.fn();
  const onCloseDrawer = vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RejectApplicationDialog
        applicationId="app-1"
        hasTelegram={props.hasTelegram ?? true}
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

describe("RejectApplicationDialog — trigger button", () => {
  it("renders trigger button labelled «Отклонить заявку», dialog initially hidden", () => {
    setup();
    expect(screen.getByTestId("reject-button")).toHaveTextContent("Отклонить заявку");
    expect(screen.queryByTestId("reject-confirm-dialog")).not.toBeInTheDocument();
  });

  it("opens dialog on trigger click", async () => {
    setup();
    await userEvent.click(screen.getByTestId("reject-button"));
    expect(screen.getByTestId("reject-confirm-dialog")).toBeInTheDocument();
  });
});

describe("RejectApplicationDialog — dialog body conditional on telegramLink", () => {
  it("shows TG body when hasTelegram=true", async () => {
    setup({ hasTelegram: true });
    await userEvent.click(screen.getByTestId("reject-button"));
    expect(
      screen.getByText(/Креатор получит уведомление в Telegram-боте/),
    ).toBeInTheDocument();
  });

  it("shows no-TG warning body when hasTelegram=false", async () => {
    setup({ hasTelegram: false });
    await userEvent.click(screen.getByTestId("reject-button"));
    expect(
      screen.getByText(/уведомление об отклонении не будет отправлено/),
    ).toBeInTheDocument();
  });
});

describe("RejectApplicationDialog — cancel", () => {
  it("closes dialog when cancel clicked", async () => {
    setup();
    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-cancel"));
    expect(screen.queryByTestId("reject-confirm-dialog")).not.toBeInTheDocument();
  });

  it("closes dialog when backdrop clicked", async () => {
    setup();
    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-backdrop"));
    expect(screen.queryByTestId("reject-confirm-dialog")).not.toBeInTheDocument();
  });
});

describe("RejectApplicationDialog — submit success", () => {
  it("calls reject api, invalidates all keys, closes dialog and drawer on 200", async () => {
    vi.mocked(rejectApplication).mockResolvedValueOnce({ data: {} });
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(rejectApplication).toHaveBeenCalledWith("app-1");
    });
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["creator-applications"],
      });
    });
    await waitFor(() => {
      expect(screen.queryByTestId("reject-confirm-dialog")).not.toBeInTheDocument();
    });
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
    expect(onApiError).not.toHaveBeenCalled();
  });
});

describe("RejectApplicationDialog — 4xx error handling", () => {
  it("on 422 NOT_REJECTABLE → onApiError(localized) + invalidate + close dialog, drawer stays", async () => {
    const code = "CREATOR_APPLICATION_NOT_REJECTABLE";
    vi.mocked(rejectApplication).mockRejectedValueOnce(new ApiError(422, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["creator-applications"],
    });
    expect(screen.queryByTestId("reject-confirm-dialog")).not.toBeInTheDocument();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 403 FORBIDDEN → onApiError + invalidate + close dialog", async () => {
    const code = "FORBIDDEN";
    vi.mocked(rejectApplication).mockRejectedValueOnce(new ApiError(403, code));
    const { onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });

  it("on 404 NOT_FOUND → onApiError + invalidate + close dialog AND drawer", async () => {
    const code = "NOT_FOUND";
    vi.mocked(rejectApplication).mockRejectedValueOnce(new ApiError(404, code));
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalled();
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
  });
});

describe("RejectApplicationDialog — network/5xx error", () => {
  it("on 500 INTERNAL_ERROR → inline error in dialog, dialog stays, no invalidate, no drawer close", async () => {
    vi.mocked(rejectApplication).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );
    const { invalidateSpy, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("reject-dialog-error")).toHaveTextContent(
        "Не удалось отклонить, попробуйте ещё раз",
      );
    });
    expect(screen.getByTestId("reject-confirm-dialog")).toBeInTheDocument();
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("on non-ApiError (network/throw) → inline error, dialog stays", async () => {
    vi.mocked(rejectApplication).mockRejectedValueOnce(new Error("network"));
    const { invalidateSpy, onCloseDrawer } = setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    await userEvent.click(screen.getByTestId("reject-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("reject-dialog-error")).toBeInTheDocument();
    });
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
  });
});

describe("RejectApplicationDialog — pending state & double-submit guard", () => {
  it("disables submit, cancel and backdrop while pending; submit text changes", async () => {
    vi.mocked(rejectApplication).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    const submit = screen.getByTestId("reject-confirm-submit");
    await userEvent.click(submit);

    await waitFor(() => {
      expect(submit).toBeDisabled();
    });
    expect(screen.getByTestId("reject-confirm-cancel")).toBeDisabled();
    expect(screen.getByTestId("reject-confirm-backdrop")).toBeDisabled();
    expect(submit).toHaveTextContent("Отклонение...");
  });

  it("ignores subsequent submit clicks while pending (double-submit guard)", async () => {
    vi.mocked(rejectApplication).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    await userEvent.click(screen.getByTestId("reject-button"));
    const submit = screen.getByTestId("reject-confirm-submit");
    await userEvent.click(submit);
    await userEvent.click(submit);
    await userEvent.click(submit);

    await waitFor(() => {
      expect(rejectApplication).toHaveBeenCalledTimes(1);
    });
  });
});
