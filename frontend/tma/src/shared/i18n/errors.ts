const errorMessages: Record<string, string> = {
  VALIDATION_ERROR: "Проверьте введённые данные",
  NOT_FOUND: "Ресурс не найден",
  FORBIDDEN: "Доступ запрещён",
  UNAUTHORIZED: "Требуется авторизация",
  CONFLICT: "Ресурс уже существует",
  INTERNAL_ERROR: "Внутренняя ошибка сервера",
  CAMPAIGN_FULL: "Кампания заполнена",
  CREATOR_NOT_FOUND: "Креатор не найден",
};

export function getErrorMessage(code: string): string {
  return errorMessages[code] ?? "Произошла ошибка";
}
