import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { calcAge } from "./age";

describe("calcAge", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-05-02T12:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("computes full years before birthday", () => {
    expect(calcAge("2008-12-31")).toBe(17);
  });

  it("computes full years after birthday", () => {
    expect(calcAge("2008-01-01")).toBe(18);
  });

  it("computes full years on exact birthday", () => {
    expect(calcAge("2008-05-02")).toBe(18);
  });
});
