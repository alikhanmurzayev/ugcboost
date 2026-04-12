import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import common from "./locales/ru/common.json";
import auth from "./locales/ru/auth.json";
import brands from "./locales/ru/brands.json";
import audit from "./locales/ru/audit.json";
import dashboard from "./locales/ru/dashboard.json";

void i18n.use(initReactI18next).init({
  lng: "ru",
  fallbackLng: "ru",
  ns: ["common", "auth", "brands", "audit", "dashboard"],
  defaultNS: "common",
  resources: {
    ru: { common, auth, brands, audit, dashboard },
  },
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
