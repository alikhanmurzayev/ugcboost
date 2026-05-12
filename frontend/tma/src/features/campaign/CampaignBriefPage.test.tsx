import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../../api/client", () => ({
  apiClient: {
    GET: vi.fn(),
    POST: vi.fn(),
  },
}));

import { apiClient } from "../../api/client";
import { CampaignBriefPage } from "./CampaignBriefPage";
import { CAMPAIGN_CREATOR_STATUS } from "../../shared/constants/campaignCreatorStatus";

const mockedGet = vi.mocked(apiClient.GET);

// Default mock keeps the visibility-block hidden for existing tests that
// assert brief content only. Visibility-specific tests override this.
beforeEach(() => {
  mockedGet.mockReset();
  mockedGet.mockResolvedValue({
    data: { status: CAMPAIGN_CREATOR_STATUS.PLANNED },
    response: { status: 200 } as Response,
  } as never);
});

afterEach(() => {
  vi.clearAllMocks();
});

// Token values mirror the keys in campaigns.ts. Repeated here verbatim so
// each test asserts the rendered page by URL the way the bot reaches it.
const QARA_BURN =
  "bda815c1775ca0a8b3b2e0002aa37af093cb65d90538db48e2034c450b974b31";
const ETHNO =
  "b75d0221430ef136ec88341595c9ce24503fa0dde9883935d69c1ad48f891a5b";
const ETHNO_BARTER =
  "7ce745a67f0c8a5567afc2b1423d4d2c38f8dd983086d66cabbc27391cbde233";
const EFW_GENERAL =
  "c559487366dbc5829e85033cc6a70233af7bcb300b7584cc2d648237caa31afe";
const KIDS =
  "bbea37590e33e6635e4bdb3150c07e29b6df15fa246df2692785366f92659a5e";

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path=":token" element={<CampaignBriefPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

// renderAndAcceptNda mounts the page and dismisses the NDA gate. The brief
// content is wrapped in `aria-hidden=true` until the user accepts the NDA,
// which excludes it from the accessibility tree — so role-based queries fail.
// All non-NDA tests here render the brief in its post-acceptance state.
async function renderAndAcceptNda(path: string) {
  const user = userEvent.setup();
  const result = renderAt(path);
  await user.click(screen.getByTestId("nda-checkbox"));
  await user.click(screen.getByTestId("nda-accept-button"));
  return result;
}

describe("CampaignBriefPage — routing & gates", () => {
  it("renders NotFoundPage for tokens that fail the secret-token format", () => {
    renderAt("/short");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Страница не найдена",
    );
  });

  it("renders the generic fallback brief for format-valid but unmapped tokens", async () => {
    await renderAndAcceptNda("/abc_def-1234567890");
    expect(screen.getByText("UGCBoost")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Приглашение в кампанию",
    );
  });

  it("blocks the brief behind the NDA dialog before user acceptance", () => {
    renderAt(`/${QARA_BURN}`);
    expect(
      screen.getByRole("dialog", { name: "Конфиденциальность брифа" }),
    ).toBeInTheDocument();
  });
});

describe("CampaignBriefPage — QARA BURN", () => {
  it("renders brand name, title, event details and reels requirements", async () => {
    await renderAndAcceptNda(`/${QARA_BURN}`);
    expect(
      screen.getByText("BURN FAMILY × CODE7212 × Arai Bektursun"),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "QARA BURN",
    );
    expect(screen.getByText("18:00–22:00")).toBeInTheDocument();
    expect(
      screen.getByText("Koktobe Hall, ул. Омаровой 35а"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Показ коллекции QARA BURN на подиуме/),
    ).toBeInTheDocument();
  });

  it("renders the about image with the configured alt text and src", async () => {
    await renderAndAcceptNda(`/${QARA_BURN}`);
    const img = screen.getByAltText("Коллекция QARA BURN") as HTMLImageElement;
    expect(img).toBeInTheDocument();
    expect(img.src).toContain("/campaigns/qara-burn/qara-burn.jpeg");
  });

  it("parses **bold** markdown inside about paragraphs as <strong>", async () => {
    await renderAndAcceptNda(`/${QARA_BURN}`);
    const matches = screen.getAllByText("13 мая на подиуме");
    expect(matches.length).toBeGreaterThan(0);
    expect(matches[0].tagName.toLowerCase()).toBe("strong");
  });

  it("does not render the Stories or Partner sections when only Reels is provided", async () => {
    await renderAndAcceptNda(`/${QARA_BURN}`);
    expect(
      screen.queryByRole("heading", { level: 3, name: "Reels" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { level: 3, name: "Stories" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { level: 2, name: /La mela/i }),
    ).not.toBeInTheDocument();
  });

  it("renders mentions notes as bullet items when notes are provided", async () => {
    await renderAndAcceptNda(`/${QARA_BURN}`);
    expect(
      screen.getByText(/Отправьте коллаб-пост на эти указанные аккаунты/),
    ).toBeInTheDocument();
  });
});

describe("CampaignBriefPage — ETHNO FASHION DAY (TrustMe variant)", () => {
  it("renders the subtitle in tagline style when subtitleAsTagline is true", async () => {
    await renderAndAcceptNda(`/${ETHNO}`);
    const subtitle = screen.getByText("Интеграция UGC boost × TrustMe");
    expect(subtitle.className).toMatch(/text-xs/);
    expect(subtitle.className).not.toMatch(/text-base/);
  });

  it("renders the designers section under the default 'Дизайнеры' heading", async () => {
    await renderAndAcceptNda(`/${ETHNO}`);
    expect(
      screen.getByRole("heading", { level: 2, name: "Дизайнеры" }),
    ).toBeInTheDocument();
    expect(screen.getByText("NURASEM")).toBeInTheDocument();
    expect(screen.getByText("Нури Рыскулова")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "@nurasem_kazakhstan" }),
    ).toHaveAttribute("href", "https://instagram.com/nurasem_kazakhstan/");
  });

  it("renders a brand-only designer row when designer name and handles are absent", async () => {
    await renderAndAcceptNda(`/${ETHNO}`);
    expect(
      screen.getByText(
        /Финалисты международного конкурса молодых дизайнеров «Жас-Өркен 2026»/,
      ),
    ).toBeInTheDocument();
  });
});

describe("CampaignBriefPage — ETHNO FASHION DAY (BARTER + La mela)", () => {
  it("renders the partner card with image, paragraphs and instagram handle link", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Информация о бренде La mela",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/казахстанский бренд профессионального ухода/),
    ).toBeInTheDocument();
    const img = screen.getByAltText(
      "Подарок La mela — шампунь и кондиционер",
    ) as HTMLImageElement;
    expect(img.src).toContain("/partners/la-mela/lamela.png");
    // @lamelacosmetics appears both in the partner card handle and in the
    // mentions section — every occurrence must point to the same instagram URL.
    const lamelaLinks = screen.getAllByRole("link", { name: "@lamelacosmetics" });
    expect(lamelaLinks.length).toBeGreaterThanOrEqual(2);
    for (const link of lamelaLinks) {
      expect(link).toHaveAttribute("href", "https://instagram.com/lamelacosmetics/");
    }
  });

  it("splits ContentCard into Reels and Stories subsections when both are provided", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(
      screen.getByRole("heading", { level: 3, name: "Reels" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 3, name: "Stories" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Атмосфера Недели моды @efw\.kz/),
    ).toBeInTheDocument();
  });

  it("renders the overridden mentions title", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Отметки и коллаб-пост",
      }),
    ).toBeInTheDocument();
  });

  it("renders the overridden designers title", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Дизайнеры ETHNO FASHION DAY",
      }),
    ).toBeInTheDocument();
  });

  it("renders the overridden aboutTitle", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Концепция 11-сезона EURASIAN FASHION WEEK 2026",
      }),
    ).toBeInTheDocument();
  });

  it("parses **bold** markdown inside reels requirement bullets", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    const strong = screen.getByText("La mela");
    expect(strong.tagName.toLowerCase()).toBe("strong");
  });

  it("renders mentions accounts as instagram links without notes when notes are absent", async () => {
    await renderAndAcceptNda(`/${ETHNO_BARTER}`);
    expect(screen.getByRole("link", { name: "@efw.kz" })).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: "@lamelacosmetics" }).length,
    ).toBeGreaterThan(0);
  });
});

describe("CampaignBriefPage — EURASIAN FASHION WEEK general", () => {
  it("renders the subtitle in default style when subtitleAsTagline is not set", async () => {
    await renderAndAcceptNda(`/${EFW_GENERAL}`);
    const subtitle = screen.getByText(
      "Показы казахстанских и зарубежных дизайнеров",
    );
    expect(subtitle.className).toMatch(/text-base/);
    expect(subtitle.className).not.toMatch(/text-xs/);
  });

  it("renders multiple instagram handles for a single brand (BLACK BURN)", async () => {
    await renderAndAcceptNda(`/${EFW_GENERAL}`);
    expect(screen.getByText("BLACK BURN")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "@burnfamily.kazakhstan" }),
    ).toHaveAttribute("href", "https://instagram.com/burnfamily.kazakhstan/");
    expect(screen.getByRole("link", { name: "@code7212" })).toHaveAttribute(
      "href",
      "https://instagram.com/code7212/",
    );
    expect(
      screen.getByRole("link", { name: "@arai_bektursun" }),
    ).toHaveAttribute("href", "https://instagram.com/arai_bektursun/");
  });

  it("renders an explicit aboutTitle override 'Информационная справка'", async () => {
    await renderAndAcceptNda(`/${EFW_GENERAL}`);
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Информационная справка",
      }),
    ).toBeInTheDocument();
  });
});

describe("CampaignBriefPage — KIDS FASHION DAY", () => {
  it("renders the reels brief as a single paragraph when briefText is set", async () => {
    await renderAndAcceptNda(`/${KIDS}`);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "KIDS FASHION DAY",
    );
    expect(
      screen.getByText(/^Снять контент с показа KIDS FASHION DAY/),
    ).toBeInTheDocument();
  });

  it("renders mentions accounts as links without notes when notes are absent", async () => {
    await renderAndAcceptNda(`/${KIDS}`);
    expect(
      screen.getByRole("link", { name: "@kaz.benetton" }),
    ).toHaveAttribute("href", "https://instagram.com/kaz.benetton/");
    expect(
      screen.getByRole("link", { name: "@boccia_kazakhstan" }),
    ).toHaveAttribute("href", "https://instagram.com/boccia_kazakhstan/");
  });

  it("does not render a designers section when none is provided", async () => {
    await renderAndAcceptNda(`/${KIDS}`);
    expect(
      screen.queryByRole("heading", { level: 2, name: "Дизайнеры" }),
    ).not.toBeInTheDocument();
  });
});

describe("CampaignBriefPage — кнопки accept/decline зависят от статуса", () => {
  it("показывает обе кнопки при статусе invited", async () => {
    mockedGet.mockResolvedValue({
      data: { status: CAMPAIGN_CREATOR_STATUS.INVITED },
      response: { status: 200 } as Response,
    } as never);
    await renderAndAcceptNda(`/${QARA_BURN}`);
    expect(
      await screen.findByTestId("campaign-accept-button"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaign-decline-button")).toBeInTheDocument();
  });

  it("скрывает кнопки при статусе signed (постпродписной архивный режим)", async () => {
    mockedGet.mockResolvedValue({
      data: { status: CAMPAIGN_CREATOR_STATUS.SIGNED },
      response: { status: 200 } as Response,
    } as never);
    await renderAndAcceptNda(`/${QARA_BURN}`);
    // бриф виден
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "QARA BURN",
    );
    // дождаться завершения GET /participation
    await waitFor(() => expect(mockedGet).toHaveBeenCalled());
    expect(screen.queryByTestId("campaign-accept-button")).toBeNull();
    expect(screen.queryByTestId("campaign-decline-button")).toBeNull();
  });
});
