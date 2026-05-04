import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import VerifyManualDialog from "./VerifyManualDialog";

vi.mock("@/api/creatorApplications", () => ({
  verifyApplicationSocialManually: vi.fn(),
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

import { verifyApplicationSocialManually } from "@/api/creatorApplications";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";

type DetailSocial = components["schemas"]["CreatorApplicationDetailSocial"];

const SOCIAL: DetailSocial = {
  id: "soc-1",
  platform: "tiktok",
  handle: "ivan",
  verified: false,
  method: undefined,
  verifiedByUserId: null,
  verifiedAt: null,
};

interface SetupProps {
  open?: boolean;
  social?: DetailSocial;
}

function setup(props: SetupProps = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
  const onClose = vi.fn();
  const onCloseDrawer = vi.fn();
  const onApiError = vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <VerifyManualDialog
        open={props.open ?? true}
        applicationId="app-1"
        social={props.social ?? SOCIAL}
        onClose={onClose}
        onCloseDrawer={onCloseDrawer}
        onApiError={onApiError}
      />
    </QueryClientProvider>,
  );
  return { ...utils, queryClient, invalidateSpy, onClose, onCloseDrawer, onApiError };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("VerifyManualDialog — open state", () => {
  it("does not render when open=false", () => {
    setup({ open: false });
    expect(screen.queryByTestId("verify-confirm-dialog")).not.toBeInTheDocument();
  });

  it("renders dialog with handle and platform when open=true", () => {
    setup();
    expect(screen.getByTestId("verify-confirm-dialog")).toBeInTheDocument();
    const body = screen.getByText(
      /Подтверждаете владение @ivan \(TikTok\)/,
    );
    expect(body).toBeInTheDocument();
  });
});

describe("VerifyManualDialog — cancel", () => {
  it("calls onClose when cancel clicked", async () => {
    const { onClose } = setup();
    await userEvent.click(screen.getByTestId("verify-confirm-cancel"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when backdrop clicked", async () => {
    const { onClose } = setup();
    await userEvent.click(screen.getByTestId("verify-confirm-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("VerifyManualDialog — submit success", () => {
  it("invalidates all keys and closes modal + drawer on 200", async () => {
    vi.mocked(verifyApplicationSocialManually).mockResolvedValueOnce({ data: {} });
    const { invalidateSpy, onClose, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await waitFor(() => {
      expect(verifyApplicationSocialManually).toHaveBeenCalledWith(
        "app-1",
        "soc-1",
      );
    });
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["creator-applications"],
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
    expect(onApiError).not.toHaveBeenCalled();
  });
});

describe("VerifyManualDialog — 4xx error handling", () => {
  it.each([
    ["CREATOR_APPLICATION_NOT_IN_VERIFICATION", 422],
    ["CREATOR_APPLICATION_TELEGRAM_NOT_LINKED", 422],
    ["CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED", 409],
  ])(
    "on %s (%i) → onApiError(localized) + invalidate + close modal, drawer stays",
    async (code, status) => {
      vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
        new ApiError(status, code),
      );
      const { invalidateSpy, onClose, onCloseDrawer, onApiError } = setup();

      await userEvent.click(screen.getByTestId("verify-confirm-submit"));

      await waitFor(() => {
        expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["creator-applications"],
      });
      expect(onClose).toHaveBeenCalledTimes(1);
      expect(onCloseDrawer).not.toHaveBeenCalled();
    },
  );

  it("on 404 SOCIAL_NOT_FOUND → onApiError(localized) + invalidate + close modal AND drawer", async () => {
    const code = "CREATOR_APPLICATION_SOCIAL_NOT_FOUND";
    vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
      new ApiError(404, code),
    );
    const { invalidateSpy, onClose, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["creator-applications"],
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
  });

  it("on 404 NOT_FOUND (generic) → onApiError(localized) + invalidate + close modal AND drawer", async () => {
    const code = "NOT_FOUND";
    vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
      new ApiError(404, code),
    );
    const { invalidateSpy, onClose, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await waitFor(() => {
      expect(onApiError).toHaveBeenCalledWith(getErrorMessage(code));
    });
    expect(invalidateSpy).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);
  });
});

describe("VerifyManualDialog — network/5xx error", () => {
  it("on 500 → inline error in modal, modal stays open, no invalidate, no drawer close", async () => {
    vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
      new ApiError(500, "INTERNAL_ERROR"),
    );
    const { invalidateSpy, onClose, onCloseDrawer, onApiError } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("verify-dialog-error")).toHaveTextContent(
        "Не удалось подтвердить, попробуйте ещё раз",
      );
    });
    expect(screen.getByTestId("verify-confirm-dialog")).toBeInTheDocument();
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(onCloseDrawer).not.toHaveBeenCalled();
    expect(onApiError).not.toHaveBeenCalled();
  });

  it("on non-ApiError (network/throw) → inline error, modal stays", async () => {
    vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
      new Error("network"),
    );
    const { invalidateSpy, onClose } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("verify-dialog-error")).toBeInTheDocument();
    });
    expect(invalidateSpy).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("VerifyManualDialog — pending state & double-submit guard", () => {
  it("disables submit, cancel and backdrop while pending; submit text changes", async () => {
    vi.mocked(verifyApplicationSocialManually).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    const submit = screen.getByTestId("verify-confirm-submit");
    await userEvent.click(submit);

    await waitFor(() => {
      expect(submit).toBeDisabled();
    });
    expect(screen.getByTestId("verify-confirm-cancel")).toBeDisabled();
    expect(screen.getByTestId("verify-confirm-backdrop")).toBeDisabled();
    expect(submit).toHaveTextContent("Подтверждение...");
  });

  it("ignores second submit click while pending (double-submit guard)", async () => {
    vi.mocked(verifyApplicationSocialManually).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    setup();

    const submit = screen.getByTestId("verify-confirm-submit");
    await userEvent.click(submit);
    await userEvent.click(submit);
    await userEvent.click(submit);

    await waitFor(() => {
      expect(verifyApplicationSocialManually).toHaveBeenCalledTimes(1);
    });
  });

  it("ignores cancel click while pending", async () => {
    vi.mocked(verifyApplicationSocialManually).mockImplementationOnce(
      () => new Promise(() => {}),
    );
    const { onClose } = setup();

    await userEvent.click(screen.getByTestId("verify-confirm-submit"));
    await waitFor(() => {
      expect(screen.getByTestId("verify-confirm-cancel")).toBeDisabled();
    });
    await userEvent.click(screen.getByTestId("verify-confirm-cancel"));
    expect(onClose).not.toHaveBeenCalled();
  });
});
