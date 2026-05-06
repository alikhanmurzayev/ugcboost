import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fireEvent } from "@testing-library/react";
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
  return { ...render(<CreatorDrawer {...merged} />), props: merged };
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
