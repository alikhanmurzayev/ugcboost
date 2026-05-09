import { describe, it, expect } from "vitest";
import { formatDateTimeShort } from "./formatDateTime";

describe("formatDateTimeShort", () => {
  it("returns em-dash for null", () => {
    expect(formatDateTimeShort(null)).toBe("—");
  });

  it("returns em-dash for undefined", () => {
    expect(formatDateTimeShort(undefined)).toBe("—");
  });

  it("returns em-dash for empty string", () => {
    expect(formatDateTimeShort("")).toBe("—");
  });

  it("returns em-dash for invalid ISO", () => {
    expect(formatDateTimeShort("not-a-date")).toBe("—");
  });

  it("formats a valid ISO without year, with day-month-time in ru locale", () => {
    const out = formatDateTimeShort("2026-05-06T14:30:00Z");
    // Локаль `ru` рендерит «6 мая, HH:MM» (без года). Час зависит от
    // часового пояса исполнителя — assert'имся на форму (day short_month,
    // HH:MM), на день/месяц и на отсутствие года, чтобы тест не флакал в CI.
    expect(out).toMatch(/^\d{1,2} \S+, \d{2}:\d{2}$/);
    expect(out).toMatch(/6 мая/);
    expect(out).not.toMatch(/2026/);
  });
});
