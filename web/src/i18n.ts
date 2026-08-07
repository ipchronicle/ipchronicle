import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import { en } from "@/locales/en";
import { zhCN } from "@/locales/zh-CN";

export const supportedLocales = ["zh-CN", "en"] as const;
export type SupportedLocale = (typeof supportedLocales)[number];

const localeStorageKey = "ipchronicle.locale";

function isSupportedLocale(value: string | null): value is SupportedLocale {
  return supportedLocales.some((locale) => locale === value);
}

export function resolveInitialLocale(): SupportedLocale {
  const stored = window.localStorage.getItem(localeStorageKey);
  if (isSupportedLocale(stored)) {
    return stored;
  }

  return window.navigator.language.toLowerCase().startsWith("zh")
    ? "zh-CN"
    : "en";
}

void i18n.use(initReactI18next).init({
  resources: {
    en,
    "zh-CN": zhCN,
  },
  lng: resolveInitialLocale(),
  fallbackLng: "en",
  supportedLngs: supportedLocales,
  interpolation: {
    escapeValue: false,
  },
  initAsync: false,
  returnNull: false,
});

function applyDocumentLocale(locale: string) {
  document.documentElement.lang = locale === "zh-CN" ? "zh-CN" : "en";
}

applyDocumentLocale(i18n.resolvedLanguage ?? "en");
i18n.on("languageChanged", applyDocumentLocale);

export async function setLocale(locale: SupportedLocale) {
  window.localStorage.setItem(localeStorageKey, locale);
  await i18n.changeLanguage(locale);
}

export default i18n;
