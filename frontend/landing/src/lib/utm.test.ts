import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { captureUTM, readUTM, UTM_KEYS } from "./utm";

interface MockStorage {
  getItem: (k: string) => string | null;
  setItem: (k: string, v: string) => void;
  removeItem: (k: string) => void;
  clear: () => void;
  data: Map<string, string>;
}

function createMockStorage(): MockStorage {
  const data = new Map<string, string>();
  return {
    data,
    getItem(k) {
      return data.get(k) ?? null;
    },
    setItem(k, v) {
      data.set(k, v);
    },
    removeItem(k) {
      data.delete(k);
    },
    clear() {
      data.clear();
    },
  };
}

function stubWindow(search: string, storage: MockStorage): void {
  vi.stubGlobal("window", {
    sessionStorage: storage,
    location: { search },
  });
}

describe("captureUTM", () => {
  let storage: MockStorage;

  beforeEach(() => {
    storage = createMockStorage();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("persists every utm_* key when present in the query", () => {
    stubWindow(
      "?utm_source=chat&utm_medium=tg&utm_campaign=spring&utm_term=ugc&utm_content=banner",
      storage,
    );

    captureUTM();

    const raw = storage.data.get("ugc_utm");
    expect(raw).toBeDefined();
    expect(JSON.parse(raw ?? "{}")).toEqual({
      utm_source: "chat",
      utm_medium: "tg",
      utm_campaign: "spring",
      utm_term: "ugc",
      utm_content: "banner",
    });
  });

  it("persists only non-empty trimmed markers from a partial query", () => {
    stubWindow("?utm_source=fb&utm_campaign=q2&utm_medium=", storage);

    captureUTM();

    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "fb",
      utm_campaign: "q2",
    });
  });

  it("trims whitespace around values before persisting", () => {
    stubWindow("?utm_source=%20telegram%20&utm_term=ugc", storage);

    captureUTM();

    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "telegram",
      utm_term: "ugc",
    });
  });

  it("leaves storage untouched when no utm_* key is in the query", () => {
    storage.setItem("ugc_utm", JSON.stringify({ utm_source: "previous" }));
    stubWindow("?other=value", storage);

    captureUTM();

    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "previous",
    });
  });

  it("overwrites previous capture under last-click", () => {
    storage.setItem(
      "ugc_utm",
      JSON.stringify({ utm_source: "old", utm_medium: "old_medium" }),
    );
    stubWindow("?utm_source=new", storage);

    captureUTM();

    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "new",
    });
  });

  it("falls back silently when sessionStorage throws and leaves the previous entry untouched", () => {
    storage.setItem(
      "ugc_utm",
      JSON.stringify({ utm_source: "previous" }),
    );
    const throwingStorage: MockStorage = {
      ...storage,
      setItem() {
        throw new Error("quota exceeded");
      },
    };
    stubWindow("?utm_source=chat", throwingStorage);

    expect(() => captureUTM()).not.toThrow();
    // The previous capture survives the throw — we never mutate storage in
    // a partial way, and the underlying Map is shared via the spread above.
    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "previous",
    });
  });

  it("ignores keys outside the canonical UTM_KEYS set", () => {
    expect(UTM_KEYS).toContain("utm_source");
    expect(UTM_KEYS).not.toContain("foo_bar");
    stubWindow("?utm_source=chat&utm_extra=ignored&foo_bar=skip", storage);

    captureUTM();

    expect(JSON.parse(storage.data.get("ugc_utm") ?? "{}")).toEqual({
      utm_source: "chat",
    });
  });
});

describe("readUTM", () => {
  let storage: MockStorage;

  beforeEach(() => {
    storage = createMockStorage();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns empty when no storage entry exists", () => {
    stubWindow("", storage);

    expect(readUTM()).toEqual({});
  });

  it("decodes a full payload back into a typed object", () => {
    storage.setItem(
      "ugc_utm",
      JSON.stringify({
        utm_source: "chat",
        utm_medium: "tg",
        utm_campaign: "spring",
        utm_term: "ugc",
        utm_content: "banner",
      }),
    );
    stubWindow("", storage);

    expect(readUTM()).toEqual({
      utm_source: "chat",
      utm_medium: "tg",
      utm_campaign: "spring",
      utm_term: "ugc",
      utm_content: "banner",
    });
  });

  it("preserves partial payloads", () => {
    storage.setItem(
      "ugc_utm",
      JSON.stringify({ utm_source: "fb", utm_campaign: "q2" }),
    );
    stubWindow("", storage);

    expect(readUTM()).toEqual({ utm_source: "fb", utm_campaign: "q2" });
  });

  it("returns empty on malformed JSON", () => {
    storage.setItem("ugc_utm", "{not-json}");
    stubWindow("", storage);

    expect(readUTM()).toEqual({});
  });

  it("ignores non-string values inside the stored object", () => {
    storage.setItem(
      "ugc_utm",
      JSON.stringify({ utm_source: 42, utm_campaign: "ok" }),
    );
    stubWindow("", storage);

    expect(readUTM()).toEqual({ utm_campaign: "ok" });
  });

  it("returns empty when the stored value is a JSON array", () => {
    // isStringRecord rejects arrays; the early-return must hit before any
    // index access so a corrupted entry never leaks into the form payload.
    storage.setItem("ugc_utm", JSON.stringify(["utm_source", "chat"]));
    stubWindow("", storage);

    expect(readUTM()).toEqual({});
  });

  it("returns empty when the stored value is the literal `null`", () => {
    storage.setItem("ugc_utm", "null");
    stubWindow("", storage);

    expect(readUTM()).toEqual({});
  });

  it("returns empty when sessionStorage.getItem throws", () => {
    const throwingStorage: MockStorage = {
      ...storage,
      getItem() {
        throw new Error("storage disabled");
      },
    };
    stubWindow("", throwingStorage);

    expect(readUTM()).toEqual({});
  });
});
