import { describe, it, expect } from "vitest";
import { getErrorMessage } from "./errors";

describe("getErrorMessage", () => {
  it("returns translated message for known codes", () => {
    expect(getErrorMessage("NOT_FOUND")).toBe("Ресурс не найден");
    expect(getErrorMessage("FORBIDDEN")).toBe("Доступ запрещён");
    expect(getErrorMessage("UNAUTHORIZED")).toBe("Требуется авторизация");
    expect(getErrorMessage("VALIDATION_ERROR")).toBe("Проверьте введённые данные");
    expect(getErrorMessage("CONFLICT")).toBe("Ресурс уже существует");
    expect(getErrorMessage("INTERNAL_ERROR")).toBe("Внутренняя ошибка сервера");
  });

  it("returns fallback for unknown code", () => {
    expect(getErrorMessage("UNKNOWN_CODE_XYZ")).toBe("Произошла ошибка");
  });
});
