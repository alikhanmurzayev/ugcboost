import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import "@/shared/i18n/config";
import CreatorDrawer from "./CreatorDrawer";
import type { CreatorAggregate, CreatorListItem } from "./types";

const PREFILL: CreatorListItem = {
  id: "id-1",
  lastName: "Иванова",
  firstName: "Анна",
  middleName: null,
  iin: "070101400001",
  birthDate: "2007-01-01",
  phone: "+77001112255",
  city: { code: "ALA", name: "Алматы", sortOrder: 10 },
  categories: [{ code: "fashion", name: "Мода", sortOrder: 1 }],
  socials: [{ platform: "instagram", handle: "anna" }],
  activeCampaignsCount: 0,
  telegramUsername: "anna",
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

const DETAIL: CreatorAggregate = {
  id: "id-1",
  iin: PREFILL.iin,
  sourceApplicationId: "src-1",
  lastName: "Иванова",
  firstName: "Анна",
  middleName: "Сергеевна",
  birthDate: "2007-01-01",
  phone: "+77001112255",
  cityCode: "ALA",
  cityName: "Алматы",
  address: "ул. Абая 1",
  categoryOtherText: null,
  telegramUserId: 42,
  telegramUsername: "anna",
  telegramFirstName: null,
  telegramLastName: null,
  socials: [
    {
      id: "s1",
      platform: "instagram",
      handle: "anna",
      verified: true,
      createdAt: "2026-04-30T12:00:00Z",
    },
  ],
  categories: [{ code: "fashion", name: "Мода" }],
  campaigns: [],
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

function renderDrawer(props: Partial<React.ComponentProps<typeof CreatorDrawer>> = {}) {
  const defaults = {
    prefill: PREFILL,
    detail: undefined,
    open: true,
    onClose: vi.fn(),
    onPrev: vi.fn(),
    onNext: vi.fn(),
    canPrev: true,
    canNext: true,
  };
  const merged = { ...defaults, ...props };
  return {
    ...render(
      <MemoryRouter>
        <CreatorDrawer {...merged} />
      </MemoryRouter>,
    ),
    props: merged,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CreatorDrawer — open/close", () => {
  it("renders nothing when open=false", () => {
    render(
      <CreatorDrawer
        prefill={PREFILL}
        detail={undefined}
        open={false}
        onClose={vi.fn()}
      />,
    );
    expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
  });

  it("renders drawer with full name when open", () => {
    renderDrawer();
    expect(screen.getByTestId("drawer")).toBeInTheDocument();
    expect(screen.getByTestId("drawer-full-name")).toHaveTextContent(
      "Иванова Анна",
    );
  });
});

describe("CreatorDrawer — prefill vs detail", () => {
  it("renders prefill data when detail not loaded", () => {
    renderDrawer();
    expect(screen.getByTestId("drawer-iin")).toHaveTextContent(PREFILL.iin);
    expect(screen.queryByTestId("drawer-address")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("drawer-source-application-id"),
    ).not.toBeInTheDocument();
  });

  it("renders detail-only fields once detail resolves", () => {
    renderDrawer({ detail: DETAIL });
    expect(screen.getByTestId("drawer-address")).toHaveTextContent("ул. Абая 1");
    expect(screen.getByTestId("drawer-source-application-id")).toHaveTextContent(
      "src-1",
    );
    expect(screen.getByTestId("drawer-middle-name")).toHaveTextContent(
      "Сергеевна",
    );
    expect(screen.getByTestId("drawer-full-name")).toHaveTextContent(
      "Иванова Анна Сергеевна",
    );
  });

  it("renders error state when detail load fails and no detail yet", () => {
    renderDrawer({ detail: undefined, isError: true });
    expect(screen.getByTestId("drawer-error")).toBeInTheDocument();
  });

  it("renders spinner when loading and detail not yet available", () => {
    renderDrawer({ detail: undefined, isLoading: true });
    expect(screen.getByTestId("drawer-detail-spinner")).toBeInTheDocument();
  });
});

describe("CreatorDrawer — keyboard navigation", () => {
  it("Escape calls onClose", () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ArrowLeft calls onPrev when canPrev=true", () => {
    const onPrev = vi.fn();
    renderDrawer({ onPrev, canPrev: true });
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  it("ArrowLeft does not call onPrev when canPrev=false", () => {
    const onPrev = vi.fn();
    renderDrawer({ onPrev, canPrev: false });
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(onPrev).not.toHaveBeenCalled();
  });

  it("ArrowRight calls onNext when canNext=true", () => {
    const onNext = vi.fn();
    renderDrawer({ onNext, canNext: true });
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("ArrowRight does not call onNext when canNext=false", () => {
    const onNext = vi.fn();
    renderDrawer({ onNext, canNext: false });
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(onNext).not.toHaveBeenCalled();
  });

  it("prev button disabled when canPrev=false", () => {
    renderDrawer({ canPrev: false });
    expect(screen.getByTestId("drawer-prev")).toBeDisabled();
  });

  it("next button disabled when canNext=false", () => {
    renderDrawer({ canNext: false });
    expect(screen.getByTestId("drawer-next")).toBeDisabled();
  });
});

describe("CreatorDrawer — close interactions", () => {
  it("clicking close button calls onClose", async () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });
    await userEvent.click(screen.getByTestId("drawer-close"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("clicking backdrop calls onClose", async () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });
    await userEvent.click(screen.getByTestId("drawer-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("clicking nav buttons triggers prev/next", async () => {
    const onPrev = vi.fn();
    const onNext = vi.fn();
    renderDrawer({ onPrev, onNext });
    await userEvent.click(screen.getByTestId("drawer-prev"));
    await userEvent.click(screen.getByTestId("drawer-next"));
    expect(onPrev).toHaveBeenCalledTimes(1);
    expect(onNext).toHaveBeenCalledTimes(1);
  });
});

describe("CreatorDrawer — telegram rendering", () => {
  it("renders @username when present", () => {
    renderDrawer({ detail: DETAIL });
    expect(screen.getByTestId("drawer-telegram-copy")).toBeInTheDocument();
  });

  it("renders firstName lastName when only TG names present", () => {
    const detail = {
      ...DETAIL,
      telegramUsername: null,
      telegramFirstName: "Анна",
      telegramLastName: "И.",
    };
    renderDrawer({ detail });
    expect(screen.getByText("Анна И.")).toBeInTheDocument();
  });

  it("renders id when neither username nor names available", () => {
    const detail = {
      ...DETAIL,
      telegramUsername: null,
      telegramFirstName: null,
      telegramLastName: null,
    };
    renderDrawer({ detail });
    expect(screen.getByText("id 42")).toBeInTheDocument();
  });
});

describe("CreatorDrawer — categoryOther", () => {
  it("renders categoryOtherText chip when present", () => {
    const detail = { ...DETAIL, categoryOtherText: "Косметика премиум" };
    renderDrawer({ detail });
    expect(screen.getByTestId("drawer-category-other-text")).toHaveTextContent(
      "Другое: Косметика премиум",
    );
  });
});

describe("CreatorDrawer — copy buttons", () => {
  let originalClipboard: typeof navigator.clipboard | undefined;

  beforeEach(() => {
    originalClipboard = navigator.clipboard;
  });

  afterEach(() => {
    if (originalClipboard) {
      Object.defineProperty(navigator, "clipboard", {
        value: originalClipboard,
        configurable: true,
        writable: true,
      });
    }
  });

  it("copies IIN to clipboard when copy button clicked", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
      writable: true,
    });

    renderDrawer({ detail: DETAIL });
    await userEvent.click(screen.getByTestId("drawer-iin-copy"));

    expect(writeText).toHaveBeenCalledWith(PREFILL.iin);
  });
});

describe("CreatorDrawer — campaigns block", () => {
  it("renders empty-state when detail has no campaigns", () => {
    renderDrawer({ detail: DETAIL });
    expect(screen.getByTestId("drawer-campaigns-empty")).toHaveTextContent(
      "Не добавлен ни в одну кампанию",
    );
    expect(
      screen.queryByTestId("drawer-campaigns-group-active"),
    ).not.toBeInTheDocument();
  });

  it("renders empty-state when detail not yet loaded (prefill has no campaigns)", () => {
    renderDrawer();
    expect(screen.getByTestId("drawer-campaigns-empty")).toBeInTheDocument();
  });

  it("groups campaigns by drawer-group order and hides empty groups", () => {
    const detail: CreatorAggregate = {
      ...DETAIL,
      campaigns: [
        { id: "camp-decl", name: "Old launch", status: "declined" },
        { id: "camp-signed", name: "Spring Drop", status: "signed" },
        { id: "camp-invited", name: "Holiday Push", status: "invited" },
      ],
    };
    renderDrawer({ detail });

    const groups = screen
      .getAllByTestId(/^drawer-campaigns-group-/)
      .map((el) => el.getAttribute("data-testid"));
    expect(groups).toEqual([
      "drawer-campaigns-group-active",
      "drawer-campaigns-group-inProgress",
      "drawer-campaigns-group-rejected",
    ]);

    const active = screen.getByTestId("drawer-campaigns-group-active");
    expect(within(active).getByTestId("drawer-campaign-camp-signed")).toHaveTextContent(
      "Spring Drop",
    );
    expect(within(active).getByTestId("drawer-campaign-camp-signed")).toHaveTextContent(
      "Подписал(а) договор",
    );

    const inProgress = screen.getByTestId("drawer-campaigns-group-inProgress");
    expect(within(inProgress).getByTestId("drawer-campaign-camp-invited")).toHaveTextContent(
      "Holiday Push",
    );

    const rejected = screen.getByTestId("drawer-campaigns-group-rejected");
    expect(within(rejected).getByTestId("drawer-campaign-camp-decl")).toHaveTextContent(
      "Old launch",
    );
  });

  it("preserves backend order within a single group", () => {
    const detail: CreatorAggregate = {
      ...DETAIL,
      campaigns: [
        { id: "camp-a", name: "Newer", status: "signed" },
        { id: "camp-b", name: "Older", status: "signed" },
      ],
    };
    renderDrawer({ detail });

    const active = screen.getByTestId("drawer-campaigns-group-active");
    const items = within(active).getAllByTestId(/^drawer-campaign-camp-/);
    expect(items.map((el) => el.getAttribute("data-testid"))).toEqual([
      "drawer-campaign-camp-a",
      "drawer-campaign-camp-b",
    ]);
  });

  it("maps all 7 statuses into the three group buckets with correct intra-group order", () => {
    // Verifies I/O matrix row 5: a creator with all 7 statuses must surface in
    // 3 groups in the prescribed order and preserve the backend's array order
    // inside every group. Backend ships campaigns sorted by created_at DESC;
    // each bucket below has two entries to also lock down intra-group order.
    const detail: CreatorAggregate = {
      ...DETAIL,
      campaigns: [
        { id: "camp-signed", name: "Signed", status: "signed" },
        { id: "camp-signing", name: "Signing", status: "signing" },
        { id: "camp-agreed", name: "Agreed", status: "agreed" },
        { id: "camp-invited", name: "Invited", status: "invited" },
        { id: "camp-planned", name: "Planned", status: "planned" },
        { id: "camp-declined", name: "Declined", status: "declined" },
        { id: "camp-signing-declined", name: "Signing Declined", status: "signing_declined" },
      ],
    };
    renderDrawer({ detail });

    expect(
      screen
        .getAllByTestId(/^drawer-campaigns-group-/)
        .map((el) => el.getAttribute("data-testid")),
    ).toEqual([
      "drawer-campaigns-group-active",
      "drawer-campaigns-group-inProgress",
      "drawer-campaigns-group-rejected",
    ]);

    const active = screen.getByTestId("drawer-campaigns-group-active");
    expect(
      within(active)
        .getAllByTestId(/^drawer-campaign-/)
        .map((el) => el.getAttribute("data-testid")),
    ).toEqual([
      "drawer-campaign-camp-signed",
      "drawer-campaign-camp-signing",
      "drawer-campaign-camp-agreed",
    ]);

    const inProgress = screen.getByTestId("drawer-campaigns-group-inProgress");
    expect(
      within(inProgress)
        .getAllByTestId(/^drawer-campaign-/)
        .map((el) => el.getAttribute("data-testid")),
    ).toEqual([
      "drawer-campaign-camp-invited",
      "drawer-campaign-camp-planned",
    ]);

    const rejected = screen.getByTestId("drawer-campaigns-group-rejected");
    expect(
      within(rejected)
        .getAllByTestId(/^drawer-campaign-/)
        .map((el) => el.getAttribute("data-testid")),
    ).toEqual([
      "drawer-campaign-camp-declined",
      "drawer-campaign-camp-signing-declined",
    ]);

    // Lock in localized labels for every status so a missing key in
    // campaigns.json surfaces as a test failure instead of a raw t() key
    // shipping to admins.
    expect(within(active).getByTestId("drawer-campaign-camp-signed")).toHaveTextContent(
      "Подписал(а) договор",
    );
    expect(within(active).getByTestId("drawer-campaign-camp-signing")).toHaveTextContent(
      "Подписывает договор",
    );
    expect(within(active).getByTestId("drawer-campaign-camp-agreed")).toHaveTextContent(
      "Согласился(лась)",
    );
    expect(within(inProgress).getByTestId("drawer-campaign-camp-invited")).toHaveTextContent(
      "Приглашён(а)",
    );
    expect(within(inProgress).getByTestId("drawer-campaign-camp-planned")).toHaveTextContent(
      "Запланирован(а)",
    );
    expect(within(rejected).getByTestId("drawer-campaign-camp-declined")).toHaveTextContent(
      "Отказался(лась)",
    );
    expect(
      within(rejected).getByTestId("drawer-campaign-camp-signing-declined"),
    ).toHaveTextContent("Отказал(ась) от договора");
  });

  it("renders a Link to /campaigns/{id} per row", () => {
    const detail: CreatorAggregate = {
      ...DETAIL,
      campaigns: [{ id: "camp-1", name: "Spring Drop", status: "signed" }],
    };
    renderDrawer({ detail });
    const link = screen.getByTestId("drawer-campaign-camp-1");
    expect(link).toHaveAttribute("href", "/campaigns/camp-1");
  });
});
