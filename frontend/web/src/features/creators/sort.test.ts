import { describe, it, expect } from "vitest";
import {
  DEFAULT_SORT,
  parseSortFromUrl,
  serializeSort,
  toggleSort,
  fieldForColumn,
  activeColumnForSort,
} from "./sort";

describe("parseSortFromUrl", () => {
  it("returns default when params absent", () => {
    expect(parseSortFromUrl(new URLSearchParams())).toEqual(DEFAULT_SORT);
  });

  it("parses valid sort/order", () => {
    const sp = new URLSearchParams("sort=birth_date&order=desc");
    expect(parseSortFromUrl(sp)).toEqual({ sort: "birth_date", order: "desc" });
  });

  it("falls back to default for unknown sort field", () => {
    const sp = new URLSearchParams("sort=garbage&order=asc");
    expect(parseSortFromUrl(sp)).toEqual({
      sort: DEFAULT_SORT.sort,
      order: "asc",
    });
  });

  it("falls back to default for unknown order", () => {
    const sp = new URLSearchParams("sort=full_name&order=sideways");
    expect(parseSortFromUrl(sp)).toEqual({
      sort: "full_name",
      order: DEFAULT_SORT.order,
    });
  });
});

describe("serializeSort", () => {
  it("omits sort/order when equal to defaults", () => {
    const sp = new URLSearchParams("sort=full_name&order=asc&id=keep");
    serializeSort(sp, { sort: "full_name", order: "asc" });
    expect(sp.has("sort")).toBe(false);
    expect(sp.has("order")).toBe(false);
    expect(sp.get("id")).toBe("keep");
  });

  it("writes sort/order when different from defaults", () => {
    const sp = new URLSearchParams();
    serializeSort(sp, { sort: "city_name", order: "desc" });
    expect(sp.get("sort")).toBe("city_name");
    expect(sp.get("order")).toBe("desc");
  });
});

describe("toggleSort", () => {
  it("switches to a new field with asc order", () => {
    expect(toggleSort({ sort: "full_name", order: "asc" }, "city_name")).toEqual({
      sort: "city_name",
      order: "asc",
    });
  });

  it("toggles asc → desc on the same field", () => {
    expect(toggleSort({ sort: "full_name", order: "asc" }, "full_name")).toEqual({
      sort: "full_name",
      order: "desc",
    });
  });

  it("toggles desc → asc on the same field", () => {
    expect(toggleSort({ sort: "full_name", order: "desc" }, "full_name")).toEqual({
      sort: "full_name",
      order: "asc",
    });
  });
});

describe("fieldForColumn", () => {
  it("returns API field for known column", () => {
    expect(fieldForColumn("fullName")).toBe("full_name");
    expect(fieldForColumn("age")).toBe("birth_date");
    expect(fieldForColumn("city")).toBe("city_name");
    expect(fieldForColumn("createdAt")).toBe("created_at");
  });

  it("returns undefined for unknown column", () => {
    expect(fieldForColumn("unknown")).toBeUndefined();
  });
});

describe("activeColumnForSort", () => {
  it("maps API field back to column key", () => {
    expect(activeColumnForSort("full_name")).toBe("fullName");
    expect(activeColumnForSort("birth_date")).toBe("age");
    expect(activeColumnForSort("city_name")).toBe("city");
    expect(activeColumnForSort("created_at")).toBe("createdAt");
  });

  it("returns undefined when API field is not mapped", () => {
    expect(activeColumnForSort("updated_at")).toBeUndefined();
  });
});
