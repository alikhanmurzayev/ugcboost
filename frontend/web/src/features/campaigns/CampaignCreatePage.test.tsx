import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import CampaignCreatePage from "./CampaignCreatePage";

vi.mock("@/api/campaigns", () => ({
  createCampaign: vi.fn(),
}));

import { createCampaign } from "@/api/campaigns";

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/campaigns/new"]}>
        <Routes>
          <Route path="/campaigns" element={<div data-testid="list-page" />} />
          <Route path="/campaigns/new" element={<CampaignCreatePage />} />
          <Route
            path="/campaigns/:id"
            element={<div data-testid="detail-page" />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, queryClient };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CampaignCreatePage — render", () => {
  it("renders header, back-link, fields and submit button", () => {
    renderPage();

    expect(screen.getByTestId("campaign-create-page")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Новая кампания" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaign-create-back")).toHaveAttribute(
      "href",
      "/campaigns",
    );
    expect(screen.getByTestId("campaign-name-input")).toBeInTheDocument();
    expect(screen.getByTestId("campaign-tma-url-input")).toBeInTheDocument();
    expect(screen.getByTestId("create-campaign-submit")).toHaveTextContent(
      "Создать кампанию",
    );
  });
});

describe("CampaignCreatePage — client validation", () => {
  it("shows both per-field errors on empty submit; createCampaign not called", async () => {
    renderPage();

    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(await screen.findByTestId("campaign-name-error")).toHaveTextContent(
      "Введите название кампании",
    );
    expect(screen.getByTestId("campaign-tma-url-error")).toHaveTextContent(
      "Введите ссылку ТЗ",
    );
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("shows only nameRequired when tmaUrl filled but name empty", async () => {
    renderPage();

    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "https://t.me/x",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(await screen.findByTestId("campaign-name-error")).toBeInTheDocument();
    expect(
      screen.queryByTestId("campaign-tma-url-error"),
    ).not.toBeInTheDocument();
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("shows only tmaUrlRequired when name filled but tmaUrl empty", async () => {
    renderPage();

    await userEvent.type(screen.getByTestId("campaign-name-input"), "promo");
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(
      await screen.findByTestId("campaign-tma-url-error"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("campaign-name-error")).not.toBeInTheDocument();
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("treats whitespace-only input as empty (trims before validation)", async () => {
    renderPage();

    await userEvent.type(screen.getByTestId("campaign-name-input"), "   ");
    await userEvent.type(screen.getByTestId("campaign-tma-url-input"), "   ");
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(await screen.findByTestId("campaign-name-error")).toBeInTheDocument();
    expect(screen.getByTestId("campaign-tma-url-error")).toBeInTheDocument();
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("error nodes carry role=alert and inputs link via aria-describedby", async () => {
    renderPage();

    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    const nameErr = await screen.findByTestId("campaign-name-error");
    expect(nameErr).toHaveAttribute("role", "alert");
    expect(screen.getByTestId("campaign-name-input")).toHaveAttribute(
      "aria-describedby",
      nameErr.id,
    );
  });
});

describe("CampaignCreatePage — happy submit", () => {
  it("calls createCampaign with trimmed values, invalidates campaigns, navigates to detail", async () => {
    const created = { data: { id: "11111111-aaaa-bbbb-cccc-222222222222" } };
    vi.mocked(createCampaign).mockResolvedValueOnce(created);

    const { queryClient } = renderPage();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    await userEvent.type(
      screen.getByTestId("campaign-name-input"),
      "  Spring promo  ",
    );
    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "  https://t.me/foo  ",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    await waitFor(() => {
      expect(createCampaign).toHaveBeenCalledTimes(1);
    });
    expect(createCampaign).toHaveBeenCalledWith({
      name: "Spring promo",
      tmaUrl: "https://t.me/foo",
    });

    await waitFor(() =>
      expect(screen.getByTestId("detail-page")).toBeInTheDocument(),
    );
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["campaigns"] });
  });
});

describe("CampaignCreatePage — server errors", () => {
  it("renders form-level error from common:errors on 409 CAMPAIGN_NAME_TAKEN; preserves field values", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(createCampaign).mockRejectedValueOnce(
      new ApiError(409, "CAMPAIGN_NAME_TAKEN"),
    );

    renderPage();

    await userEvent.type(
      screen.getByTestId("campaign-name-input"),
      "Existing",
    );
    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "https://t.me/x",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    const err = await screen.findByTestId("create-campaign-error");
    expect(err).toHaveAttribute("role", "alert");
    expect(err).toHaveTextContent(/Кампания с таким названием уже есть/);
    expect(screen.getByTestId("campaign-name-input")).toHaveValue("Existing");
    expect(screen.getByTestId("campaign-tma-url-input")).toHaveValue(
      "https://t.me/x",
    );
  });

  it("falls back to unknown text on non-ApiError (network)", async () => {
    vi.mocked(createCampaign).mockRejectedValueOnce(new Error("network down"));

    renderPage();

    await userEvent.type(screen.getByTestId("campaign-name-input"), "Any");
    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "https://t.me/x",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(await screen.findByTestId("create-campaign-error")).toHaveTextContent(
      "Произошла ошибка",
    );
  });

  it("falls back to unknown text on ApiError with unmapped code", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(createCampaign).mockRejectedValueOnce(
      new ApiError(500, "TOTALLY_UNMAPPED_CODE"),
    );

    renderPage();

    await userEvent.type(screen.getByTestId("campaign-name-input"), "Any");
    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "https://t.me/x",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    expect(await screen.findByTestId("create-campaign-error")).toHaveTextContent(
      "Произошла ошибка",
    );
  });
});

describe("CampaignCreatePage — submit guard", () => {
  it("disables submit while pending and renders submittingButton text", async () => {
    vi.mocked(createCampaign).mockImplementation(() => new Promise(() => {}));

    renderPage();

    await userEvent.type(screen.getByTestId("campaign-name-input"), "Promo");
    await userEvent.type(
      screen.getByTestId("campaign-tma-url-input"),
      "https://t.me/x",
    );
    await userEvent.click(screen.getByTestId("create-campaign-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("create-campaign-submit")).toBeDisabled();
    });
    expect(screen.getByTestId("create-campaign-submit")).toHaveTextContent(
      "Создаётся…",
    );

    // Second click while pending must be a no-op — disabled blocks userEvent
    // and the handler's `if (isSubmitting || isPending) return` defends
    // against any race where the button briefly appears clickable.
    await userEvent.click(screen.getByTestId("create-campaign-submit"));
    expect(createCampaign).toHaveBeenCalledTimes(1);
  });
});
