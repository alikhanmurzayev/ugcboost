import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ApplicationDetail } from "../types";
import ApplicationDrawer from "./ApplicationDrawer";

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

const APP_ID = "app-uuid-1";
const IG_ID = "soc-ig-1";
const TT_ID = "soc-tt-2";

function makeDetail(
  overrides: Partial<ApplicationDetail> = {},
): ApplicationDetail {
  return {
    id: APP_ID,
    verificationCode: "UGC-123456",
    lastName: "Иванов",
    firstName: "Иван",
    middleName: null,
    iin: "080101300000",
    birthDate: "2008-01-01",
    phone: "+77001112233",
    address: null,
    city: { code: "ALA", name: "Алматы", sortOrder: 10 },
    categories: [],
    categoryOtherText: null,
    socials: [
      {
        id: IG_ID,
        platform: "instagram",
        handle: "ivan_auto",
        verified: true,
        method: "auto",
        verifiedByUserId: null,
        verifiedAt: "2026-04-30T12:00:00Z",
      },
      {
        id: TT_ID,
        platform: "tiktok",
        handle: "ivan_tt",
        verified: false,
        method: undefined,
        verifiedByUserId: null,
        verifiedAt: null,
      },
    ],
    consents: [],
    telegramLink: {
      telegramUserId: 1,
      telegramUsername: "ivan",
      telegramFirstName: null,
      telegramLastName: null,
      linkedAt: "2026-04-30T11:00:00Z",
    },
    telegramBotUrl: `https://t.me/ugcboost_test_bot?start=${APP_ID}`,
    status: "verification",
    createdAt: "2026-04-30T10:00:00Z",
    updatedAt: "2026-04-30T12:00:00Z",
    ...overrides,
  } as ApplicationDetail;
}

function renderDrawer(detail: ApplicationDetail = makeDetail()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const onClose = vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <ApplicationDrawer
        application={detail}
        open
        onClose={onClose}
      />
    </QueryClientProvider>,
  );
  return { ...utils, queryClient, onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ApplicationDrawer — Socials rendered through SocialAdminRow", () => {
  it("shows verified-auto badge for IG and verify button for TT (TG linked)", () => {
    renderDrawer();

    expect(screen.getByTestId(`verified-badge-${IG_ID}`)).toHaveTextContent(
      "Подтверждено · авто",
    );
    expect(
      screen.queryByTestId(`verify-social-${IG_ID}`),
    ).not.toBeInTheDocument();

    const ttButton = screen.getByTestId(`verify-social-${TT_ID}`);
    expect(ttButton).toBeEnabled();
    expect(
      screen.queryByTestId(`verify-social-${TT_ID}-disabled-hint`),
    ).not.toBeInTheDocument();
  });

  it("disables verify button + shows hint when TG NOT linked", () => {
    renderDrawer(makeDetail({ telegramLink: null }));

    const button = screen.getByTestId(`verify-social-${TT_ID}`);
    expect(button).toBeDisabled();
    expect(
      screen.getByTestId(`verify-social-${TT_ID}-disabled-hint`),
    ).toBeInTheDocument();
  });
});

describe("ApplicationDrawer — verify modal wiring", () => {
  it("opens VerifyManualDialog with the right handle/platform on verify click", async () => {
    renderDrawer();

    expect(
      screen.queryByTestId("verify-confirm-dialog"),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId(`verify-social-${TT_ID}`));

    expect(screen.getByTestId("verify-confirm-dialog")).toBeInTheDocument();
    expect(
      screen.getByText(/Подтверждаете владение @ivan_tt \(TikTok\)/),
    ).toBeInTheDocument();
  });

  it("does NOT open dialog on click of disabled (no-TG) button", async () => {
    renderDrawer(makeDetail({ telegramLink: null }));

    await userEvent.click(screen.getByTestId(`verify-social-${TT_ID}`));

    expect(
      screen.queryByTestId("verify-confirm-dialog"),
    ).not.toBeInTheDocument();
  });

  it("closes dialog on cancel without affecting drawer", async () => {
    const { onClose } = renderDrawer();

    await userEvent.click(screen.getByTestId(`verify-social-${TT_ID}`));
    expect(screen.getByTestId("verify-confirm-dialog")).toBeInTheDocument();

    await userEvent.click(screen.getByTestId("verify-confirm-cancel"));

    expect(
      screen.queryByTestId("verify-confirm-dialog"),
    ).not.toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });
});
