import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import CampaignDetailPage from "./CampaignDetailPage";
import { campaignKeys } from "@/shared/constants/queryKeys";

vi.mock("@/api/campaigns", () => ({
  getCampaign: vi.fn(),
  updateCampaign: vi.fn(),
}));

vi.mock("@/api/campaignCreators", () => ({
  listCampaignCreators: vi.fn(),
}));

vi.mock("@/api/creators", () => ({
  listCreators: vi.fn(),
  getCreator: vi.fn(),
}));

import { getCampaign, updateCampaign } from "@/api/campaigns";
import { listCampaignCreators } from "@/api/campaignCreators";
import { listCreators, getCreator } from "@/api/creators";

const ID = "11111111-1111-1111-1111-111111111111";
const ID_OTHER = "22222222-2222-2222-2222-222222222222";

const FIXTURE_LIVE = {
  id: ID,
  name: "Spring Promo",
  tmaUrl: "https://t.me/ugcboost_bot/app?startapp=spring",
  isDeleted: false,
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-05-01T08:00:00Z",
} as const;

const FIXTURE_DELETED = {
  ...FIXTURE_LIVE,
  id: ID_OTHER,
  name: "Old Drop",
  isDeleted: true,
} as const;

function renderPage(id: string = ID, search = "") {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/campaigns/${id}${search}`]}>
        <Routes>
          <Route
            path="/campaigns"
            element={<div data-testid="list-page" />}
          />
          <Route
            path="/campaigns/:campaignId"
            element={<CampaignDetailPage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, queryClient };
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listCampaignCreators).mockResolvedValue([]);
  vi.mocked(listCreators).mockResolvedValue({
    data: { items: [], total: 0, page: 1, perPage: 200 },
  });
});

describe("CampaignDetailPage — loading & error", () => {
  it("renders Spinner while GET pending", () => {
    vi.mocked(getCampaign).mockImplementation(() => new Promise(() => {}));

    renderPage();

    expect(screen.getByTestId("campaign-detail-page")).toBeInTheDocument();
    expect(screen.queryByTestId("campaign-detail-title")).not.toBeInTheDocument();
  });

  it("renders dedicated not-found state on 404 (no form rendered)", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(getCampaign).mockRejectedValueOnce(
      new ApiError(404, "CAMPAIGN_NOT_FOUND"),
    );

    renderPage();

    expect(
      await screen.findByTestId("campaign-detail-not-found"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Кампания не найдена" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaign-detail-back")).toHaveAttribute(
      "href",
      "/campaigns",
    );
    expect(screen.queryByTestId("campaign-edit-form")).not.toBeInTheDocument();
    expect(screen.queryByTestId("campaign-detail-title")).not.toBeInTheDocument();
  });

  it("renders ErrorState with retry on non-404 failure; retry refires getCampaign", async () => {
    vi.mocked(getCampaign).mockRejectedValueOnce(new Error("boom"));
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    expect(await screen.findByTestId("error-state")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("error-retry-button"));

    expect(
      await screen.findByTestId("campaign-detail-title"),
    ).toHaveTextContent("Spring Promo");
    expect(getCampaign).toHaveBeenCalledTimes(2);
  });
});

describe("CampaignDetailPage — view mode", () => {
  it("renders title, name, tmaUrl as link, dates, edit button enabled (live)", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    expect(
      await screen.findByTestId("campaign-detail-title"),
    ).toHaveTextContent("Spring Promo");
    expect(screen.getByTestId("campaign-detail-name")).toHaveTextContent(
      "Spring Promo",
    );
    const tmaLink = screen.getByTestId("campaign-detail-tma-url");
    expect(tmaLink).toHaveAttribute("href", FIXTURE_LIVE.tmaUrl);
    expect(tmaLink).toHaveAttribute("target", "_blank");
    expect(tmaLink).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByTestId("campaign-detail-created-at")).toBeInTheDocument();
    expect(screen.getByTestId("campaign-detail-updated-at")).toBeInTheDocument();

    const editBtn = screen.getByTestId("campaign-edit-button");
    expect(editBtn).toBeEnabled();
    expect(
      screen.queryByTestId("campaign-detail-deleted-badge"),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("campaign-edit-form")).not.toBeInTheDocument();
  });

  it("shows deleted badge and disables edit button with tooltip on soft-deleted", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_DELETED });

    renderPage(FIXTURE_DELETED.id);

    expect(
      await screen.findByTestId("campaign-detail-deleted-badge"),
    ).toHaveTextContent("Удалена");
    const editBtn = screen.getByTestId("campaign-edit-button");
    expect(editBtn).toBeDisabled();
    expect(editBtn).toHaveAttribute(
      "title",
      "Удалённую кампанию редактировать нельзя",
    );
  });

  it("back-link in view mode points to /campaigns", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    const back = await screen.findByTestId("campaign-detail-back");
    expect(back).toHaveAttribute("href", "/campaigns");
  });

  it("renders tmaUrl as plain text (not anchor) when scheme is unsafe", async () => {
    // javascript: URLs would otherwise execute arbitrary JS on click —
    // defence-in-depth from the frontend. Backend storage may contain such
    // URLs because campaign validation does not enforce a scheme.
    vi.mocked(getCampaign).mockResolvedValueOnce({
      data: { ...FIXTURE_LIVE, tmaUrl: "javascript:alert(1)" },
    });

    renderPage();

    const node = await screen.findByTestId("campaign-detail-tma-url");
    // Anchor would have an href attribute and tag name `A`. Text-fallback is
    // a plain <span> with no href.
    expect(node.tagName).toBe("SPAN");
    expect(node).not.toHaveAttribute("href");
    expect(node).toHaveTextContent("javascript:alert(1)");
  });

  it("renders dash for invalid ISO date strings", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({
      data: { ...FIXTURE_LIVE, updatedAt: "not-a-date" },
    });

    renderPage();

    expect(
      await screen.findByTestId("campaign-detail-updated-at"),
    ).toHaveTextContent("—");
  });
});

describe("CampaignDetailPage — enter & cancel edit", () => {
  it("clicking edit shows form prefilled with current values; cancel restores view discarding edits", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    await userEvent.click(await screen.findByTestId("campaign-edit-button"));

    const nameInput = screen.getByTestId(
      "campaign-edit-name-input",
    ) as HTMLInputElement;
    const tmaInput = screen.getByTestId(
      "campaign-edit-tma-url-input",
    ) as HTMLInputElement;
    expect(nameInput.value).toBe("Spring Promo");
    expect(tmaInput.value).toBe(FIXTURE_LIVE.tmaUrl);
    expect(screen.getByTestId("campaign-edit-submit")).toHaveTextContent(
      "Сохранить",
    );
    expect(screen.getByTestId("campaign-edit-cancel")).toBeInTheDocument();
    // Edit-button hidden while editing
    expect(
      screen.queryByTestId("campaign-edit-button"),
    ).not.toBeInTheDocument();

    // Type into the field then cancel — local edit must be dropped
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "Renamed");
    await userEvent.click(screen.getByTestId("campaign-edit-cancel"));

    expect(screen.queryByTestId("campaign-edit-form")).not.toBeInTheDocument();
    expect(screen.getByTestId("campaign-detail-name")).toHaveTextContent(
      "Spring Promo",
    );
    expect(updateCampaign).not.toHaveBeenCalled();
  });
});

describe("CampaignDetailPage — client validation in edit", () => {
  async function enterEditAndClear() {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    renderPage();
    await userEvent.click(await screen.findByTestId("campaign-edit-button"));
  }

  it("shows both inline errors when both fields cleared on submit; no PATCH", async () => {
    await enterEditAndClear();
    await userEvent.clear(screen.getByTestId("campaign-edit-name-input"));
    await userEvent.clear(screen.getByTestId("campaign-edit-tma-url-input"));

    await userEvent.click(screen.getByTestId("campaign-edit-submit"));

    expect(
      await screen.findByTestId("campaign-edit-name-error"),
    ).toHaveTextContent("Введите название кампании");
    expect(screen.getByTestId("campaign-edit-tma-url-error")).toHaveTextContent(
      "Введите ссылку ТЗ",
    );
    expect(updateCampaign).not.toHaveBeenCalled();
  });

  it("treats whitespace-only as empty (trim before validation)", async () => {
    await enterEditAndClear();
    const nameInput = screen.getByTestId("campaign-edit-name-input");
    const tmaInput = screen.getByTestId("campaign-edit-tma-url-input");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "   ");
    await userEvent.clear(tmaInput);
    await userEvent.type(tmaInput, "   ");

    await userEvent.click(screen.getByTestId("campaign-edit-submit"));

    expect(
      await screen.findByTestId("campaign-edit-name-error"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaign-edit-tma-url-error")).toBeInTheDocument();
    expect(updateCampaign).not.toHaveBeenCalled();
  });

  it("error nodes carry role=alert and both inputs link via aria-describedby", async () => {
    await enterEditAndClear();
    await userEvent.clear(screen.getByTestId("campaign-edit-name-input"));
    await userEvent.clear(screen.getByTestId("campaign-edit-tma-url-input"));
    await userEvent.click(screen.getByTestId("campaign-edit-submit"));

    const nameErr = await screen.findByTestId("campaign-edit-name-error");
    expect(nameErr).toHaveAttribute("role", "alert");
    expect(screen.getByTestId("campaign-edit-name-input")).toHaveAttribute(
      "aria-describedby",
      nameErr.id,
    );

    const tmaErr = screen.getByTestId("campaign-edit-tma-url-error");
    expect(tmaErr).toHaveAttribute("role", "alert");
    expect(screen.getByTestId("campaign-edit-tma-url-input")).toHaveAttribute(
      "aria-describedby",
      tmaErr.id,
    );
  });
});

describe("CampaignDetailPage — happy save", () => {
  it("calls updateCampaign with trimmed values, invalidates detail+all, returns to view, refetch shows new values", async () => {
    const updated = {
      ...FIXTURE_LIVE,
      name: "Spring Promo 2",
      tmaUrl: "https://t.me/ugcboost_bot/app?startapp=spring2",
      updatedAt: "2026-05-02T08:00:00Z",
    };
    vi.mocked(getCampaign)
      .mockResolvedValueOnce({ data: FIXTURE_LIVE })
      .mockResolvedValue({ data: updated });
    vi.mocked(updateCampaign).mockResolvedValueOnce(undefined);

    const { queryClient } = renderPage();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await userEvent.click(await screen.findByTestId("campaign-edit-button"));
    const nameInput = screen.getByTestId("campaign-edit-name-input");
    const tmaInput = screen.getByTestId("campaign-edit-tma-url-input");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "  Spring Promo 2  ");
    await userEvent.clear(tmaInput);
    await userEvent.type(
      tmaInput,
      "  https://t.me/ugcboost_bot/app?startapp=spring2  ",
    );
    await userEvent.click(screen.getByTestId("campaign-edit-submit"));

    await waitFor(() => {
      expect(updateCampaign).toHaveBeenCalledTimes(1);
    });
    expect(updateCampaign).toHaveBeenCalledWith(ID, {
      name: "Spring Promo 2",
      tmaUrl: "https://t.me/ugcboost_bot/app?startapp=spring2",
    });

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignKeys.detail(ID),
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: campaignKeys.all(),
    });

    // Returned to view; refetch surfaces fresh values
    expect(
      await screen.findByTestId("campaign-detail-name"),
    ).toHaveTextContent("Spring Promo 2");
    expect(screen.queryByTestId("campaign-edit-form")).not.toBeInTheDocument();
    // First call rendered the page; subsequent calls are refetches triggered
    // by the two invalidations. We don't assert an exact count — react-query
    // may collapse them — but at minimum a refetch must have happened.
    expect(vi.mocked(getCampaign).mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});

describe("CampaignDetailPage — server errors in edit", () => {
  async function enterEditAndSubmit(name = "Existing") {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    renderPage();
    await userEvent.click(await screen.findByTestId("campaign-edit-button"));
    const nameInput = screen.getByTestId("campaign-edit-name-input");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, name);
    await userEvent.click(screen.getByTestId("campaign-edit-submit"));
  }

  it("renders form-level error from common:errors on 409 CAMPAIGN_NAME_TAKEN; preserves field values; stays in edit", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(updateCampaign).mockRejectedValueOnce(
      new ApiError(409, "CAMPAIGN_NAME_TAKEN"),
    );

    await enterEditAndSubmit("Existing");

    const err = await screen.findByTestId("campaign-edit-error");
    expect(err).toHaveAttribute("role", "alert");
    expect(err).toHaveTextContent(/Кампания с таким названием уже есть/);
    expect(screen.getByTestId("campaign-edit-name-input")).toHaveValue(
      "Existing",
    );
    // Still in edit mode
    expect(screen.getByTestId("campaign-edit-form")).toBeInTheDocument();
  });

  it("maps PATCH 404 race to form-level error from getErrorMessage", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(updateCampaign).mockRejectedValueOnce(
      new ApiError(404, "CAMPAIGN_NOT_FOUND"),
    );

    await enterEditAndSubmit("Renamed");

    expect(
      await screen.findByTestId("campaign-edit-error"),
    ).toHaveTextContent("Кампания не найдена.");
    expect(screen.getByTestId("campaign-edit-form")).toBeInTheDocument();
  });

  it("maps PATCH 422 CAMPAIGN_NAME_TOO_LONG to form-level error from getErrorMessage", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(updateCampaign).mockRejectedValueOnce(
      new ApiError(422, "CAMPAIGN_NAME_TOO_LONG"),
    );

    await enterEditAndSubmit("Renamed");

    expect(
      await screen.findByTestId("campaign-edit-error"),
    ).toHaveTextContent(/Название кампании слишком длинное/);
    expect(screen.getByTestId("campaign-edit-form")).toBeInTheDocument();
  });

  it("falls back to unknown text on non-ApiError (network down)", async () => {
    vi.mocked(updateCampaign).mockRejectedValueOnce(new Error("network"));

    await enterEditAndSubmit("AnyName");

    expect(
      await screen.findByTestId("campaign-edit-error"),
    ).toHaveTextContent("Произошла ошибка");
  });

  it("falls back to unknown text on ApiError with unmapped code", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(updateCampaign).mockRejectedValueOnce(
      new ApiError(500, "TOTALLY_UNMAPPED_CODE"),
    );

    await enterEditAndSubmit("AnyName");

    expect(
      await screen.findByTestId("campaign-edit-error"),
    ).toHaveTextContent("Произошла ошибка");
  });
});

describe("CampaignDetailPage — submit guard", () => {
  it("disables submit while pending; second click is a no-op", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    vi.mocked(updateCampaign).mockImplementation(() => new Promise(() => {}));

    renderPage();

    await userEvent.click(await screen.findByTestId("campaign-edit-button"));
    await userEvent.click(screen.getByTestId("campaign-edit-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("campaign-edit-submit")).toBeDisabled();
    });
    expect(screen.getByTestId("campaign-edit-submit")).toHaveTextContent(
      "Сохранение…",
    );

    // Cancel button is also disabled while submitting
    expect(screen.getByTestId("campaign-edit-cancel")).toBeDisabled();

    // Second click while pending must be a no-op — disabled blocks userEvent
    // and the handler's `if (isSubmitting || isPending) return` defends
    // against any race where the button briefly appears clickable.
    await userEvent.click(screen.getByTestId("campaign-edit-submit"));
    expect(updateCampaign).toHaveBeenCalledTimes(1);
  });
});

const CREATOR_ID = "33333333-3333-3333-3333-333333333333";

const FIXTURE_CC = {
  id: "cc-1",
  campaignId: ID,
  creatorId: CREATOR_ID,
  status: "planned" as const,
  invitedAt: null,
  invitedCount: 0,
  remindedAt: null,
  remindedCount: 0,
  decidedAt: null,
  createdAt: "2026-05-07T12:00:00Z",
  updatedAt: "2026-05-07T12:00:00Z",
};

const FIXTURE_CREATOR_LIST_ITEM = {
  id: CREATOR_ID,
  lastName: "Иванова",
  firstName: "Анна",
  middleName: null,
  iin: "070101400001",
  birthDate: "2007-01-01",
  phone: "+77001112255",
  city: { code: "ALA", name: "Алматы", sortOrder: 10 },
  categories: [{ code: "fashion", name: "Мода", sortOrder: 1 }],
  socials: [{ platform: "instagram" as const, handle: "anna" }],
  telegramUsername: "anna",
  createdAt: "2026-04-30T12:00:00Z",
  updatedAt: "2026-04-30T12:00:00Z",
};

const FIXTURE_CREATOR_DETAIL = {
  id: CREATOR_ID,
  iin: FIXTURE_CREATOR_LIST_ITEM.iin,
  sourceApplicationId: "44444444-4444-4444-4444-444444444444",
  lastName: FIXTURE_CREATOR_LIST_ITEM.lastName,
  firstName: FIXTURE_CREATOR_LIST_ITEM.firstName,
  middleName: null,
  birthDate: FIXTURE_CREATOR_LIST_ITEM.birthDate,
  phone: FIXTURE_CREATOR_LIST_ITEM.phone,
  cityCode: "ALA",
  cityName: "Алматы",
  address: "ул. Абая 1",
  categoryOtherText: null,
  telegramUserId: 42,
  telegramUsername: FIXTURE_CREATOR_LIST_ITEM.telegramUsername,
  telegramFirstName: null,
  telegramLastName: null,
  socials: [
    {
      id: "s1",
      platform: "instagram" as const,
      handle: "anna",
      verified: true,
      createdAt: "2026-04-30T12:00:00Z",
    },
  ],
  categories: [{ code: "fashion", name: "Мода" }],
  createdAt: FIXTURE_CREATOR_LIST_ITEM.createdAt,
  updatedAt: FIXTURE_CREATOR_LIST_ITEM.updatedAt,
};

describe("CampaignDetailPage — campaign creators section", () => {
  it("renders the section on a live campaign and fires A3", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    await screen.findByTestId("campaign-creators-section");
    await waitFor(() => {
      expect(listCampaignCreators).toHaveBeenCalledWith(ID);
    });
  });

  it("does not render the section on a soft-deleted campaign and does not fire A3", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_DELETED });

    renderPage(FIXTURE_DELETED.id);

    await screen.findByTestId("campaign-detail-deleted-badge");
    expect(
      screen.queryByTestId("campaign-creators-section"),
    ).not.toBeInTheDocument();
    expect(listCampaignCreators).not.toHaveBeenCalled();
  });

  it("renders rows when A3 + listCreators succeed and skips listCreators on empty A3", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([FIXTURE_CC]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [FIXTURE_CREATOR_LIST_ITEM],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });

    renderPage();

    expect(await screen.findByTestId(`row-${CREATOR_ID}`)).toBeInTheDocument();
    expect(screen.getByTestId("campaign-creators-counter")).toHaveTextContent(
      "1 в кампании",
    );
  });

  it("disables the Add button with tooltip", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });

    renderPage();

    const btn = await screen.findByTestId("campaign-creators-add-button");
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("title", "Появится в следующем PR");
  });
});

describe("CampaignDetailPage — creator drawer via URL", () => {
  it("opens drawer when ?creatorId is present on mount and fetches detail", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([FIXTURE_CC]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [FIXTURE_CREATOR_LIST_ITEM],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(getCreator).mockResolvedValueOnce({
      data: FIXTURE_CREATOR_DETAIL,
    });

    renderPage(ID, `?creatorId=${CREATOR_ID}`);

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() => {
      expect(getCreator).toHaveBeenCalledWith(CREATOR_ID);
    });
    await waitFor(() =>
      expect(screen.getByTestId("drawer-full-name")).toHaveTextContent(
        "Иванова Анна",
      ),
    );
  });

  it("opens drawer on row click and writes creatorId to URL", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([FIXTURE_CC]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [FIXTURE_CREATOR_LIST_ITEM],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(getCreator).mockResolvedValueOnce({
      data: FIXTURE_CREATOR_DETAIL,
    });

    renderPage();

    await userEvent.click(await screen.findByTestId(`row-${CREATOR_ID}`));

    expect(await screen.findByTestId("drawer")).toBeInTheDocument();
    await waitFor(() => {
      expect(getCreator).toHaveBeenCalledWith(CREATOR_ID);
    });
  });

  it("closes drawer when close button clicked", async () => {
    vi.mocked(getCampaign).mockResolvedValueOnce({ data: FIXTURE_LIVE });
    vi.mocked(listCampaignCreators).mockResolvedValueOnce([FIXTURE_CC]);
    vi.mocked(listCreators).mockResolvedValueOnce({
      data: {
        items: [FIXTURE_CREATOR_LIST_ITEM],
        total: 1,
        page: 1,
        perPage: 200,
      },
    });
    vi.mocked(getCreator).mockResolvedValueOnce({
      data: FIXTURE_CREATOR_DETAIL,
    });

    renderPage(ID, `?creatorId=${CREATOR_ID}`);

    const close = await screen.findByTestId("drawer-close");
    await userEvent.click(close);

    await waitFor(() => {
      expect(screen.queryByTestId("drawer")).not.toBeInTheDocument();
    });
  });
});
