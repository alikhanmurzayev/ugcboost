import { describe, it, expect } from "vitest";
import {
  parseFilters,
  writeFilters,
  clearFilters,
  isFilterActive,
  toListInput,
} from "./filters";

describe("parseFilters", () => {
  it("returns defaults for empty params", () => {
    expect(parseFilters(new URLSearchParams())).toEqual({
      search: undefined,
      showDeleted: false,
    });
  });

  it("parses search and showDeleted=true", () => {
    const sp = new URLSearchParams("q=promo&showDeleted=true");
    expect(parseFilters(sp)).toEqual({ search: "promo", showDeleted: true });
  });

  it("treats showDeleted other than 'true' as false", () => {
    const sp = new URLSearchParams("showDeleted=1");
    expect(parseFilters(sp).showDeleted).toBe(false);
  });
});

describe("writeFilters", () => {
  it("URL roundtrip preserves values", () => {
    const original = { search: "promo", showDeleted: true };
    const sp = new URLSearchParams();
    writeFilters(sp, original);
    expect(parseFilters(sp)).toEqual(original);
  });

  it("removes keys for empty/false values", () => {
    const sp = new URLSearchParams("q=stale&showDeleted=true");
    writeFilters(sp, { search: undefined, showDeleted: false });
    expect(sp.has("q")).toBe(false);
    expect(sp.has("showDeleted")).toBe(false);
  });

  it("trims whitespace-only search to nothing (no q in URL)", () => {
    const sp = new URLSearchParams("q=stale");
    writeFilters(sp, { search: "   ", showDeleted: false });
    expect(sp.has("q")).toBe(false);
  });

  it("trims surrounding whitespace from search before writing", () => {
    const sp = new URLSearchParams();
    writeFilters(sp, { search: "  promo  ", showDeleted: false });
    expect(sp.get("q")).toBe("promo");
  });
});

describe("clearFilters", () => {
  it("removes filter keys but keeps others", () => {
    const sp = new URLSearchParams("q=x&showDeleted=true&id=keep&page=2");
    clearFilters(sp);
    expect(sp.has("q")).toBe(false);
    expect(sp.has("showDeleted")).toBe(false);
    expect(sp.get("id")).toBe("keep");
    expect(sp.get("page")).toBe("2");
  });
});

describe("isFilterActive", () => {
  it("returns false when neither search nor showDeleted active", () => {
    expect(isFilterActive({ showDeleted: false })).toBe(false);
  });

  it("returns true when search set", () => {
    expect(isFilterActive({ search: "x", showDeleted: false })).toBe(true);
  });

  it("returns true when showDeleted=true", () => {
    expect(isFilterActive({ showDeleted: true })).toBe(true);
  });
});

describe("toListInput", () => {
  const baseOpts = {
    sort: { sort: "created_at" as const, order: "desc" as const },
    page: 1,
    perPage: 50,
  };

  it("emits required fields + isDeleted=false when showDeleted off", () => {
    const body = toListInput({ showDeleted: false }, baseOpts);
    expect(body).toEqual({
      page: 1,
      perPage: 50,
      sort: "created_at",
      order: "desc",
      isDeleted: false,
    });
  });

  it("omits isDeleted when showDeleted on (means: include all)", () => {
    const body = toListInput({ showDeleted: true }, baseOpts);
    expect(body).toEqual({
      page: 1,
      perPage: 50,
      sort: "created_at",
      order: "desc",
    });
    expect(body.isDeleted).toBeUndefined();
  });

  it("forwards trimmed non-empty search", () => {
    const body = toListInput({ search: "  promo  ", showDeleted: false }, baseOpts);
    expect(body.search).toBe("promo");
  });

  it("trims and ignores blank search", () => {
    const body = toListInput({ search: "   ", showDeleted: false }, baseOpts);
    expect(body.search).toBeUndefined();
  });
});
