import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ApplicationDetail } from "../types";
import ApplicationActions from "./ApplicationActions";
import ApplicationDrawer from "./ApplicationDrawer";

vi.mock("@/api/creatorApplications", () => ({
  rejectApplication: vi.fn(),
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

function makeDetail(
  overrides: Partial<ApplicationDetail> = {},
): ApplicationDetail {
  return {
    id: "app-1",
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
    socials: [],
    consents: [],
    telegramLink: {
      telegramUserId: 1,
      telegramUsername: "ivan",
      telegramFirstName: null,
      telegramLastName: null,
      linkedAt: "2026-04-30T11:00:00Z",
    },
    telegramBotUrl: "https://t.me/bot?start=app-1",
    status: "verification",
    createdAt: "2026-04-30T10:00:00Z",
    updatedAt: "2026-04-30T12:00:00Z",
    ...overrides,
  } as ApplicationDetail;
}

function renderInDrawer(detail: ApplicationDetail | undefined) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ApplicationDrawer
        application={detail}
        open
        onClose={vi.fn()}
        footer={<ApplicationActions application={detail} />}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ApplicationActions — switch by status", () => {
  it.each(["verification", "moderation"] as const)(
    "renders reject button for %s status",
    (status) => {
      renderInDrawer(makeDetail({ status }));
      expect(screen.getByTestId("application-actions")).toBeInTheDocument();
      expect(screen.getByTestId("reject-button")).toBeInTheDocument();
    },
  );

  it.each([
    "awaiting_contract",
    "contract_sent",
    "signed",
    "rejected",
    "withdrawn",
  ] as const)("renders nothing for %s status", (status) => {
    renderInDrawer(makeDetail({ status }));
    expect(screen.queryByTestId("application-actions")).not.toBeInTheDocument();
    expect(screen.queryByTestId("reject-button")).not.toBeInTheDocument();
  });

  it("renders nothing when application is undefined", () => {
    renderInDrawer(undefined);
    expect(screen.queryByTestId("application-actions")).not.toBeInTheDocument();
  });
});
