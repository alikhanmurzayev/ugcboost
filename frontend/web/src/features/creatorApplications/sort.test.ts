import { describe, it, expect } from "vitest";
import {
  parseSortFromUrl,
  serializeSort,
  toggleSort,
  fieldForColumn,
  DEFAULT_SORT,
} from "./sort";

describe("parseSortFromUrl", () => {
  it("returns default when params absent", () => {
    expect(parseSortFromUrl(new URLSearchParams())).toEqual(DEFAULT_SORT);
  });

  it("parses valid sort + order", () => {
    expect(
      parseSortFromUrl(new URLSearchParams("sort=full_name&order=asc")),
    ).toEqual({ sort: "full_name", order: "asc" });
  });

  it("falls back to default for unknown sort field", () => {
    expect(
      parseSortFromUrl(new URLSearchParams("sort=invalid&order=asc")),
    ).toEqual({ sort: "created_at", order: "asc" });
  });

  it("falls back to default for unknown order", () => {
    expect(
      parseSortFromUrl(new URLSearchParams("sort=full_name&order=sideways")),
    ).toEqual({ sort: "full_name", order: "desc" });
  });
});

describe("serializeSort", () => {
  it("removes keys when state equals default", () => {
    const sp = new URLSearchParams("sort=created_at&order=desc&page=2");
    serializeSort(sp, DEFAULT_SORT);
    expect(sp.has("sort")).toBe(false);
    expect(sp.has("order")).toBe(false);
    expect(sp.get("page")).toBe("2");
  });

  it("writes keys when state differs from default", () => {
    const sp = new URLSearchParams();
    serializeSort(sp, { sort: "full_name", order: "asc" });
    expect(sp.get("sort")).toBe("full_name");
    expect(sp.get("order")).toBe("asc");
  });
});

describe("toggleSort", () => {
  it("switches to desc when changing field", () => {
    expect(toggleSort({ sort: "created_at", order: "asc" }, "full_name")).toEqual(
      { sort: "full_name", order: "desc" },
    );
  });

  it("flips order on same field", () => {
    expect(toggleSort({ sort: "created_at", order: "desc" }, "created_at")).toEqual(
      { sort: "created_at", order: "asc" },
    );
    expect(toggleSort({ sort: "created_at", order: "asc" }, "created_at")).toEqual(
      { sort: "created_at", order: "desc" },
    );
  });
});

describe("fieldForColumn", () => {
  it("maps known UI columns to API fields", () => {
    expect(fieldForColumn("fullName")).toBe("full_name");
    expect(fieldForColumn("submittedAt")).toBe("created_at");
    expect(fieldForColumn("city")).toBe("city_name");
    expect(fieldForColumn("birthDate")).toBe("birth_date");
  });

  it("returns undefined for non-sortable columns", () => {
    expect(fieldForColumn("index")).toBeUndefined();
    expect(fieldForColumn("socials")).toBeUndefined();
    expect(fieldForColumn("categories")).toBeUndefined();
  });

  it("respects custom column map override", () => {
    const map = { hoursInStage: "updated_at" as const };
    expect(fieldForColumn("hoursInStage", map)).toBe("updated_at");
    expect(fieldForColumn("fullName", map)).toBeUndefined();
  });
});

describe("parseSortFromUrl with custom defaults", () => {
  it("falls back to provided defaults when params absent", () => {
    expect(
      parseSortFromUrl(new URLSearchParams(), {
        sort: "updated_at",
        order: "asc",
      }),
    ).toEqual({ sort: "updated_at", order: "asc" });
  });

  it("returns parsed values when valid, ignoring custom defaults", () => {
    expect(
      parseSortFromUrl(new URLSearchParams("sort=full_name&order=desc"), {
        sort: "updated_at",
        order: "asc",
      }),
    ).toEqual({ sort: "full_name", order: "desc" });
  });
});

describe("serializeSort with custom defaults", () => {
  it("removes keys when state equals provided defaults", () => {
    const sp = new URLSearchParams("sort=updated_at&order=asc&page=3");
    serializeSort(
      sp,
      { sort: "updated_at", order: "asc" },
      { sort: "updated_at", order: "asc" },
    );
    expect(sp.has("sort")).toBe(false);
    expect(sp.has("order")).toBe(false);
    expect(sp.get("page")).toBe("3");
  });

  it("writes keys when state differs from provided defaults", () => {
    const sp = new URLSearchParams();
    serializeSort(
      sp,
      { sort: "created_at", order: "desc" },
      { sort: "updated_at", order: "asc" },
    );
    expect(sp.get("sort")).toBe("created_at");
    expect(sp.get("order")).toBe("desc");
  });
});
