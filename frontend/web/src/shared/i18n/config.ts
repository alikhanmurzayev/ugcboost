import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import common from "./locales/ru/common.json";
import auth from "./locales/ru/auth.json";
import brands from "./locales/ru/brands.json";
import audit from "./locales/ru/audit.json";
import dashboard from "./locales/ru/dashboard.json";

// Prototype namespaces — Aidana's brand-cabinet mock. Kept under prefixed names
// (prototype_*) so they don't collide with future real namespaces.
import prototypeCampaigns from "@/_prototype/locales/ru/campaigns.json";
import prototypeCreatorApplications from "@/_prototype/locales/ru/creatorApplications.json";
import prototypeCreators from "@/_prototype/locales/ru/creators.json";

void i18n.use(initReactI18next).init({
  lng: "ru",
  fallbackLng: "ru",
  ns: [
    "common",
    "auth",
    "brands",
    "audit",
    "dashboard",
    "prototype_campaigns",
    "prototype_creatorApplications",
    "prototype_creators",
  ],
  defaultNS: "common",
  resources: {
    ru: {
      common,
      auth,
      brands,
      audit,
      dashboard,
      prototype_campaigns: prototypeCampaigns,
      prototype_creatorApplications: prototypeCreatorApplications,
      prototype_creators: prototypeCreators,
    },
  },
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
