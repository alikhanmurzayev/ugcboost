const errorMessages: Record<string, string> = {
  VALIDATION_ERROR: "Проверьте введённые данные",
  NOT_FOUND: "Ресурс не найден",
  FORBIDDEN: "Доступ запрещён",
  UNAUTHORIZED: "Требуется авторизация",
  CONFLICT: "Ресурс уже существует",
  INTERNAL_ERROR: "Внутренняя ошибка сервера",
  CAMPAIGN_FULL: "Кампания заполнена",
  CREATOR_NOT_FOUND: "Креатор не найден",
  CAMPAIGN_NOT_FOUND: "Кампания не найдена.",
  TMA_FORBIDDEN: "У вас нет приглашения на эту кампанию.",
  CAMPAIGN_CREATOR_NOT_INVITED:
    "Вы не приглашены в эту кампанию. Дождитесь приглашения от админа.",
  CAMPAIGN_CREATOR_DECLINED_NEED_REINVITE:
    "Вы уже отказались. Чтобы согласиться, дождитесь повторного приглашения.",
  CAMPAIGN_CREATOR_ALREADY_AGREED:
    "Вы уже согласились на участие в этой кампании.",
};

export function getErrorMessage(code: string): string {
  return errorMessages[code] ?? "Произошла ошибка";
}

export function decisionErrorMessage(code: string): string {
  return errorMessages[code] ?? "Не удалось сохранить решение. Попробуйте ещё раз.";
}
