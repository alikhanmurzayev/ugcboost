import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import * as React from "react";
import type { ApplicationDetail } from "../types";
import ApplicationDrawer from "./ApplicationDrawer";
import { useDrawerContext } from "./drawerContext";

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

function renderDrawer(
  detail: ApplicationDetail = makeDetail(),
  options: { footer?: React.ReactNode } = {},
) {
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
        footer={options.footer}
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

describe("ApplicationDrawer — footer slot", () => {
  it("does NOT render drawer-footer when footer prop is omitted", () => {
    renderDrawer();
    expect(screen.queryByTestId("drawer-footer")).not.toBeInTheDocument();
  });

  it("renders drawer-footer with the passed ReactNode when footer prop is set", () => {
    renderDrawer(makeDetail(), {
      footer: <span data-testid="custom-footer-content">my action</span>,
    });
    expect(screen.getByTestId("drawer-footer")).toBeInTheDocument();
    expect(screen.getByTestId("custom-footer-content")).toHaveTextContent(
      "my action",
    );
  });
});

describe("ApplicationDrawer — UTM section", () => {
  it("renders no utm-section when every utm field is null", () => {
    renderDrawer(makeDetail());

    expect(screen.queryByTestId("utm-section")).not.toBeInTheDocument();
  });

  it("renders five labelled rows when every utm field carries a value", () => {
    renderDrawer(
      makeDetail({
        utmSource: "telegram_chat",
        utmMedium: "tg",
        utmCampaign: "spring2026",
        utmTerm: "ugc",
        utmContent: "banner",
      }),
    );

    expect(screen.getByTestId("utm-section")).toBeInTheDocument();
    expect(screen.getByText("Источник трафика")).toBeInTheDocument();
    expect(screen.getByTestId("utm-source-value")).toHaveTextContent(
      "telegram_chat",
    );
    expect(screen.getByTestId("utm-medium-value")).toHaveTextContent("tg");
    expect(screen.getByTestId("utm-campaign-value")).toHaveTextContent(
      "spring2026",
    );
    expect(screen.getByTestId("utm-term-value")).toHaveTextContent("ugc");
    expect(screen.getByTestId("utm-content-value")).toHaveTextContent("banner");
  });

  it("renders only the populated rows on a partial utm payload", () => {
    renderDrawer(
      makeDetail({
        utmSource: "fb",
        utmCampaign: "q2",
      }),
    );

    expect(screen.getByTestId("utm-section")).toBeInTheDocument();
    expect(screen.getByTestId("utm-source-value")).toHaveTextContent("fb");
    expect(screen.getByTestId("utm-campaign-value")).toHaveTextContent("q2");
    expect(screen.queryByTestId("utm-medium-value")).not.toBeInTheDocument();
    expect(screen.queryByTestId("utm-term-value")).not.toBeInTheDocument();
    expect(screen.queryByTestId("utm-content-value")).not.toBeInTheDocument();
  });
});

describe("ApplicationDrawer — DrawerContext + apiError banner", () => {
  function FooterErrorTrigger() {
    const { onApiError } = useDrawerContext();
    return (
      <button
        type="button"
        data-testid="raise-api-error"
        onClick={() => onApiError("боль")}
      >
        raise
      </button>
    );
  }

  it("provides DrawerContext to footer; setting onApiError shows banner with that text", async () => {
    renderDrawer(makeDetail(), { footer: <FooterErrorTrigger /> });

    expect(screen.queryByTestId("drawer-api-error")).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId("raise-api-error"));

    expect(screen.getByTestId("drawer-api-error")).toHaveTextContent("боль");
  });

  it("verify dialog 4xx error surfaces in unified drawer-api-error banner", async () => {
    const { ApiError } = await import("@/api/client");
    const { verifyApplicationSocialManually } = await import(
      "@/api/creatorApplications"
    );
    vi.mocked(verifyApplicationSocialManually).mockRejectedValueOnce(
      new ApiError(422, "CREATOR_APPLICATION_NOT_IN_VERIFICATION"),
    );

    renderDrawer();

    await userEvent.click(screen.getByTestId(`verify-social-${TT_ID}`));
    await userEvent.click(screen.getByTestId("verify-confirm-submit"));

    await screen.findByTestId("drawer-api-error");
    expect(screen.getByTestId("drawer-api-error")).toHaveTextContent(
      "Заявка уже не на этапе верификации",
    );
  });
});
