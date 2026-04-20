import i18n from "./config";

export function getErrorMessage(code: string): string {
  const key = `common:errors.${code}`;
  if (i18n.exists(key)) {
    return i18n.t(key);
  }
  return i18n.t("common:errors.unknown");
}
