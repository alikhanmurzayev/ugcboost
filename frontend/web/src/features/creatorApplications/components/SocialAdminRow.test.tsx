import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { components } from "@/api/generated/schema";
import SocialAdminRow from "./SocialAdminRow";

type DetailSocial = components["schemas"]["CreatorApplicationDetailSocial"];

const SOCIAL_ID = "soc-uuid-1";

function makeSocial(overrides: Partial<DetailSocial> = {}): DetailSocial {
  return {
    id: SOCIAL_ID,
    platform: "instagram",
    handle: "ivan",
    verified: false,
    method: undefined,
    verifiedByUserId: null,
    verifiedAt: null,
    ...overrides,
  };
}

describe("SocialAdminRow", () => {
  it("renders auto-verified badge when verified=true / method=auto", () => {
    render(
      <SocialAdminRow
        social={makeSocial({
          verified: true,
          method: "auto",
          verifiedAt: "2026-04-30T12:00:00Z",
        })}
        telegramLinked
        onVerifyClick={vi.fn()}
      />,
    );

    expect(screen.getByTestId(`verified-badge-${SOCIAL_ID}`)).toHaveTextContent(
      "Подтверждено · авто",
    );
    expect(
      screen.queryByTestId(`verify-social-${SOCIAL_ID}`),
    ).not.toBeInTheDocument();
  });

  it("renders manual-verified badge when verified=true / method=manual", () => {
    render(
      <SocialAdminRow
        social={makeSocial({
          verified: true,
          method: "manual",
          verifiedAt: "2026-05-01T08:30:00Z",
        })}
        telegramLinked
        onVerifyClick={vi.fn()}
      />,
    );

    expect(screen.getByTestId(`verified-badge-${SOCIAL_ID}`)).toHaveTextContent(
      "Подтверждено · вручную",
    );
  });

  it("renders enabled verify button when unverified and TG linked", async () => {
    const onVerifyClick = vi.fn();
    const social = makeSocial();
    render(
      <SocialAdminRow
        social={social}
        telegramLinked
        onVerifyClick={onVerifyClick}
      />,
    );

    const button = screen.getByTestId(`verify-social-${SOCIAL_ID}`);
    expect(button).toBeEnabled();
    expect(
      screen.queryByTestId(`verify-social-${SOCIAL_ID}-disabled-hint`),
    ).not.toBeInTheDocument();

    await userEvent.click(button);
    expect(onVerifyClick).toHaveBeenCalledTimes(1);
    expect(onVerifyClick).toHaveBeenCalledWith(social);
  });

  it("renders disabled verify button + hint when unverified and TG NOT linked", async () => {
    const onVerifyClick = vi.fn();
    render(
      <SocialAdminRow
        social={makeSocial()}
        telegramLinked={false}
        onVerifyClick={onVerifyClick}
      />,
    );

    const button = screen.getByTestId(`verify-social-${SOCIAL_ID}`);
    expect(button).toBeDisabled();
    expect(
      screen.getByTestId(`verify-social-${SOCIAL_ID}-disabled-hint`),
    ).toHaveTextContent("Сначала креатор должен привязать Telegram");

    await userEvent.click(button);
    expect(onVerifyClick).not.toHaveBeenCalled();
  });
});
