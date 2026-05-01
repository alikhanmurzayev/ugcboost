// Prototype-local error message resolver. The prototype only ever sees mock-
// thrown ApiError values, so the lookup table is intentionally tiny — extend
// only if a new mock branch starts throwing a new code.
const PROTOTYPE_ERROR_MESSAGES: Record<string, string> = {
  unknown: "Что-то пошло не так",
  validation_failed: "Проверьте введённые данные",
  not_found: "Не найдено",
};

export function getErrorMessage(code: string): string {
  return PROTOTYPE_ERROR_MESSAGES[code] ?? PROTOTYPE_ERROR_MESSAGES.unknown!;
}
