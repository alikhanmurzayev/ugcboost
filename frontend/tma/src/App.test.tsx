import { describe, it, expect } from "vitest";

import App from "./App";

describe("App", () => {
  it("exports a renderable component (smoke)", () => {
    expect(App).toBeTypeOf("function");
  });
});
