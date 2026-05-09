import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { DeclinedView } from "./DeclinedView";

describe("DeclinedView", () => {
  it("does not render the already-decided banner by default", () => {
    render(<DeclinedView />);
    expect(
      screen.queryByTestId("tma-already-decided-banner"),
    ).not.toBeInTheDocument();
  });

  it("renders the banner when alreadyDecided is true", () => {
    render(<DeclinedView alreadyDecided />);
    const banner = screen.getByTestId("tma-already-decided-banner");
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent("уже отказывались");
  });
});
