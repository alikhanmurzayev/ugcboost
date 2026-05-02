import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  parseFilters,
  writeFilters,
  clearFilters,
  isFilterActive,
  countActive,
  toListInput,
  calcAge,
} from "./filters";

describe("parseFilters", () => {
  it("returns empty/default values for empty params", () => {
    const f = parseFilters(new URLSearchParams());
    expect(f).toEqual({
      search: undefined,
      dateFrom: undefined,
      dateTo: undefined,
      cities: [],
      ageFrom: undefined,
      ageTo: undefined,
      categories: [],
      telegramLinked: undefined,
    });
  });

  it("parses all fields from URL", () => {
    const sp = new URLSearchParams(
      "q=Иван&dateFrom=2026-04-01&dateTo=2026-05-01&cities=ALA,AST&ageFrom=18&ageTo=35&categories=fashion,beauty&telegramLinked=true",
    );
    expect(parseFilters(sp)).toEqual({
      search: "Иван",
      dateFrom: "2026-04-01",
      dateTo: "2026-05-01",
      cities: ["ALA", "AST"],
      ageFrom: 18,
      ageTo: 35,
      categories: ["fashion", "beauty"],
      telegramLinked: true,
    });
  });

  it("parses telegramLinked false explicitly", () => {
    const f = parseFilters(new URLSearchParams("telegramLinked=false"));
    expect(f.telegramLinked).toBe(false);
  });

  it("ignores malformed telegramLinked value", () => {
    const f = parseFilters(new URLSearchParams("telegramLinked=maybe"));
    expect(f.telegramLinked).toBeUndefined();
  });

  it("ignores malformed date params", () => {
    const sp = new URLSearchParams("dateFrom=not-a-date&dateTo=2026-13-99");
    const f = parseFilters(sp);
    expect(f.dateFrom).toBeUndefined();
    expect(f.dateTo).toBeUndefined();
  });

  it("ignores malformed age params", () => {
    const sp = new URLSearchParams("ageFrom=abc&ageTo=-5");
    const f = parseFilters(sp);
    expect(f.ageFrom).toBeUndefined();
    expect(f.ageTo).toBeUndefined();
  });

  it("strips empty CSV entries", () => {
    const sp = new URLSearchParams("cities=ALA,,AST&categories=,fashion,");
    const f = parseFilters(sp);
    expect(f.cities).toEqual(["ALA", "AST"]);
    expect(f.categories).toEqual(["fashion"]);
  });
});

describe("writeFilters", () => {
  it("URL roundtrip preserves all values", () => {
    const original = {
      search: "Иван",
      dateFrom: "2026-04-01",
      dateTo: "2026-05-01",
      cities: ["ALA", "AST"],
      ageFrom: 20,
      ageTo: 30,
      categories: ["fashion"],
      telegramLinked: false,
    };
    const sp = new URLSearchParams();
    writeFilters(sp, original);
    expect(parseFilters(sp)).toEqual(original);
  });

  it("removes keys for undefined/empty values", () => {
    const sp = new URLSearchParams(
      "q=stale&cities=stale&telegramLinked=true",
    );
    writeFilters(sp, {
      search: undefined,
      cities: [],
      categories: [],
      telegramLinked: undefined,
    });
    expect(sp.has("q")).toBe(false);
    expect(sp.has("cities")).toBe(false);
    expect(sp.has("telegramLinked")).toBe(false);
  });
});

describe("clearFilters", () => {
  it("removes all known filter keys but keeps others", () => {
    const sp = new URLSearchParams(
      "q=x&dateFrom=2026-01-01&cities=ALA&telegramLinked=false&id=keep-me&page=2",
    );
    clearFilters(sp);
    expect(sp.has("q")).toBe(false);
    expect(sp.has("dateFrom")).toBe(false);
    expect(sp.has("cities")).toBe(false);
    expect(sp.has("telegramLinked")).toBe(false);
    expect(sp.get("id")).toBe("keep-me");
    expect(sp.get("page")).toBe("2");
  });
});

describe("isFilterActive", () => {
  const empty = {
    cities: [] as string[],
    categories: [] as string[],
  };

  it("returns false for empty filters", () => {
    expect(isFilterActive(empty)).toBe(false);
  });

  it("returns true for any non-empty field", () => {
    expect(isFilterActive({ ...empty, search: "x" })).toBe(true);
    expect(isFilterActive({ ...empty, dateFrom: "2026-01-01" })).toBe(true);
    expect(isFilterActive({ ...empty, dateTo: "2026-01-01" })).toBe(true);
    expect(isFilterActive({ ...empty, cities: ["ALA"] })).toBe(true);
    expect(isFilterActive({ ...empty, categories: ["x"] })).toBe(true);
    expect(isFilterActive({ ...empty, ageFrom: 18 })).toBe(true);
    expect(isFilterActive({ ...empty, ageTo: 30 })).toBe(true);
    expect(isFilterActive({ ...empty, telegramLinked: true })).toBe(true);
    expect(isFilterActive({ ...empty, telegramLinked: false })).toBe(true);
  });
});

describe("countActive", () => {
  it("counts each field group once", () => {
    expect(countActive({ cities: [], categories: [] })).toBe(0);
    expect(
      countActive({
        dateFrom: "2026-01-01",
        cities: ["ALA"],
        categories: ["x"],
        ageFrom: 18,
        telegramLinked: false,
      }),
    ).toBe(5);
    expect(
      countActive({
        dateFrom: "2026-01-01",
        dateTo: "2026-02-01",
        cities: [],
        categories: [],
      }),
    ).toBe(1);
    expect(
      countActive({
        cities: [],
        categories: [],
        telegramLinked: true,
      }),
    ).toBe(1);
  });
});

describe("toListInput", () => {
  const baseOpts = {
    statuses: ["verification" as const],
    sort: { sort: "created_at" as const, order: "desc" as const },
    page: 1,
    perPage: 50,
  };

  it("emits required fields with empty filters", () => {
    const body = toListInput(
      { cities: [], categories: [] },
      baseOpts,
    );
    expect(body).toEqual({
      page: 1,
      perPage: 50,
      sort: "created_at",
      order: "desc",
      statuses: ["verification"],
    });
  });

  it("matches AC body for typical URL filters", () => {
    const body = toListInput(
      {
        search: "Иван",
        dateFrom: "2026-04-01",
        cities: ["ALA"],
        categories: [],
      },
      baseOpts,
    );
    expect(body).toEqual({
      page: 1,
      perPage: 50,
      sort: "created_at",
      order: "desc",
      statuses: ["verification"],
      search: "Иван",
      dateFrom: "2026-04-01T00:00:00.000Z",
      cities: ["ALA"],
    });
  });

  it("converts dateTo to end-of-day UTC", () => {
    const body = toListInput(
      { dateTo: "2026-05-01", cities: [], categories: [] },
      baseOpts,
    );
    expect(body.dateTo).toBe("2026-05-01T23:59:59.999Z");
  });

  it("trims and ignores blank search", () => {
    const body = toListInput(
      { search: "   ", cities: [], categories: [] },
      baseOpts,
    );
    expect(body.search).toBeUndefined();
  });

  it("emits age bounds and categories when present", () => {
    const body = toListInput(
      {
        cities: [],
        categories: ["fashion", "beauty"],
        ageFrom: 18,
        ageTo: 35,
      },
      baseOpts,
    );
    expect(body.categories).toEqual(["fashion", "beauty"]);
    expect(body.ageFrom).toBe(18);
    expect(body.ageTo).toBe(35);
  });

  it("emits telegramLinked=true when set", () => {
    const body = toListInput(
      { cities: [], categories: [], telegramLinked: true },
      baseOpts,
    );
    expect(body.telegramLinked).toBe(true);
  });

  it("emits telegramLinked=false when set", () => {
    const body = toListInput(
      { cities: [], categories: [], telegramLinked: false },
      baseOpts,
    );
    expect(body.telegramLinked).toBe(false);
  });

  it("omits telegramLinked when undefined", () => {
    const body = toListInput({ cities: [], categories: [] }, baseOpts);
    expect("telegramLinked" in body).toBe(false);
  });
});

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
