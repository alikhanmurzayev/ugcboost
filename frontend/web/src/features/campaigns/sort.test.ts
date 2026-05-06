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
    const sp = new URLSearchParams("sort=name&order=asc");
    expect(parseSortFromUrl(sp)).toEqual({ sort: "name", order: "asc" });
  });

  it("falls back to default for unknown sort field", () => {
    const sp = new URLSearchParams("sort=full_name&order=asc");
    expect(parseSortFromUrl(sp)).toEqual({
      sort: DEFAULT_SORT.sort,
      order: "asc",
    });
  });

  it("falls back to default for unknown order", () => {
    const sp = new URLSearchParams("sort=name&order=sideways");
    expect(parseSortFromUrl(sp)).toEqual({
      sort: "name",
      order: DEFAULT_SORT.order,
    });
  });
});

describe("serializeSort", () => {
  it("omits sort/order when equal to defaults", () => {
    const sp = new URLSearchParams("sort=created_at&order=desc&id=keep");
    serializeSort(sp, { sort: "created_at", order: "desc" });
    expect(sp.has("sort")).toBe(false);
    expect(sp.has("order")).toBe(false);
    expect(sp.get("id")).toBe("keep");
  });

  it("writes sort/order when different from defaults", () => {
    const sp = new URLSearchParams();
    serializeSort(sp, { sort: "name", order: "asc" });
    expect(sp.get("sort")).toBe("name");
    expect(sp.get("order")).toBe("asc");
  });
});

describe("toggleSort", () => {
  it("switches to a new field with asc order", () => {
    expect(toggleSort({ sort: "created_at", order: "desc" }, "name")).toEqual({
      sort: "name",
      order: "asc",
    });
  });

  it("toggles asc → desc on the same field", () => {
    expect(toggleSort({ sort: "name", order: "asc" }, "name")).toEqual({
      sort: "name",
      order: "desc",
    });
  });

  it("toggles desc → asc on the same field", () => {
    expect(toggleSort({ sort: "name", order: "desc" }, "name")).toEqual({
      sort: "name",
      order: "asc",
    });
  });
});

describe("fieldForColumn", () => {
  it("returns API field for known column", () => {
    expect(fieldForColumn("name")).toBe("name");
    expect(fieldForColumn("createdAt")).toBe("created_at");
  });

  it("returns undefined for unknown column", () => {
    expect(fieldForColumn("tmaUrl")).toBeUndefined();
  });
});

describe("activeColumnForSort", () => {
  it("maps API field back to column key", () => {
    expect(activeColumnForSort("name")).toBe("name");
    expect(activeColumnForSort("created_at")).toBe("createdAt");
  });

  it("returns undefined when API field is not mapped", () => {
    expect(activeColumnForSort("updated_at")).toBeUndefined();
  });
});
