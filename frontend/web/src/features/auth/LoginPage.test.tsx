import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "./LoginPage";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
}));

vi.mock("@/stores/auth", () => ({
  useAuthStore: vi.fn((selector) => {
    const state = { user: null, token: null, setAuth: vi.fn(), clearAuth: vi.fn() };
    return selector(state);
  }),
}));

function renderLogin() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders login form", () => {
    renderLogin();

    expect(screen.getByTestId("login-form")).toBeInTheDocument();
    expect(screen.getByTestId("email-input")).toBeInTheDocument();
    expect(screen.getByTestId("password-input")).toBeInTheDocument();
    expect(screen.getByTestId("login-button")).toBeInTheDocument();
  });

  it("shows error on failed login", async () => {
    const { login } = await import("@/api/auth");
    const { ApiError } = await import("@/api/client");
    vi.mocked(login).mockRejectedValue(new ApiError(401, "UNAUTHORIZED"));

    renderLogin();

    await userEvent.type(screen.getByTestId("email-input"), "test@test.com");
    await userEvent.type(screen.getByTestId("password-input"), "wrongpass");
    await userEvent.click(screen.getByTestId("login-button"));

    expect(await screen.findByTestId("login-error")).toHaveTextContent("Неверный email или пароль");
  });

  it("disables button while loading", async () => {
    const { login } = await import("@/api/auth");
    vi.mocked(login).mockImplementation(() => new Promise(() => {})); // never resolves

    renderLogin();

    await userEvent.type(screen.getByTestId("email-input"), "test@test.com");
    await userEvent.type(screen.getByTestId("password-input"), "password123");
    await userEvent.click(screen.getByTestId("login-button"));

    expect(screen.getByTestId("login-button")).toBeDisabled();
  });
});
