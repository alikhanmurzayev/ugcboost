import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@/shared/i18n/config";
import RemoveCreatorConfirm from "./RemoveCreatorConfirm";

const baseProps = {
  open: true,
  creatorName: "Иванова Анна",
  isLoading: false,
  onClose: () => {},
  onConfirm: () => {},
};

describe("RemoveCreatorConfirm", () => {
  it("returns null when open=false", () => {
    render(<RemoveCreatorConfirm {...baseProps} open={false} />);

    expect(screen.queryByTestId("remove-creator-confirm")).not.toBeInTheDocument();
  });

  it("renders title, message with creator name, and dialog role", () => {
    render(<RemoveCreatorConfirm {...baseProps} />);

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText("Удалить креатора?")).toBeInTheDocument();
    expect(
      screen.getByText(/Иванова Анна/),
    ).toBeInTheDocument();
  });

  it("calls onConfirm when the confirm button is clicked", async () => {
    const onConfirm = vi.fn();
    render(<RemoveCreatorConfirm {...baseProps} onConfirm={onConfirm} />);

    await userEvent.click(screen.getByTestId("remove-creator-confirm-submit"));

    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the cancel button is clicked", async () => {
    const onClose = vi.fn();
    render(<RemoveCreatorConfirm {...baseProps} onClose={onClose} />);

    await userEvent.click(screen.getByTestId("remove-creator-confirm-cancel"));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the backdrop is clicked", async () => {
    const onClose = vi.fn();
    render(<RemoveCreatorConfirm {...baseProps} onClose={onClose} />);

    await userEvent.click(
      screen.getByTestId("remove-creator-confirm-backdrop"),
    );

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when Escape is pressed", async () => {
    const onClose = vi.fn();
    render(<RemoveCreatorConfirm {...baseProps} onClose={onClose} />);

    await userEvent.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("disables both buttons while isLoading=true and shows loading label on submit", () => {
    render(<RemoveCreatorConfirm {...baseProps} isLoading />);

    expect(screen.getByTestId("remove-creator-confirm-cancel")).toBeDisabled();
    expect(screen.getByTestId("remove-creator-confirm-submit")).toBeDisabled();
    expect(screen.getByTestId("remove-creator-confirm-submit")).toHaveTextContent(
      "Удаление…",
    );
  });

  it("ignores Escape while isLoading=true", async () => {
    const onClose = vi.fn();
    render(<RemoveCreatorConfirm {...baseProps} onClose={onClose} isLoading />);

    await userEvent.keyboard("{Escape}");

    expect(onClose).not.toHaveBeenCalled();
  });

  it("renders error message with role=alert when error is provided", () => {
    render(
      <RemoveCreatorConfirm {...baseProps} error="Не удалось удалить, попробуйте ещё раз" />,
    );

    const alert = screen.getByTestId("remove-creator-confirm-error");
    expect(alert).toHaveAttribute("role", "alert");
    expect(alert).toHaveTextContent(/Не удалось удалить/);
  });

  it("does not render error block when error is undefined", () => {
    render(<RemoveCreatorConfirm {...baseProps} />);

    expect(
      screen.queryByTestId("remove-creator-confirm-error"),
    ).not.toBeInTheDocument();
  });
});
