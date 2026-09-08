"use client";

import { createContext, useContext, useEffect, useMemo, useState, useTransition, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { isLocale, languageCookie, languageNames, localeCookie, locales, type Locale } from "./locale";
import { translate, translateKnownMessage, type MessageKey, type MessageValues } from "./messages";
import styles from "./language-select.module.css";

const LocaleContext = createContext<Locale>("en");

export function LocaleProvider({ locale, children }: { locale: Locale; children: ReactNode }) {
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return <LocaleContext.Provider value={locale}>{children}</LocaleContext.Provider>;
}

export function useTranslations() {
  const locale = useContext(LocaleContext);
  return useMemo(() => ({
    locale,
    t: (key: MessageKey, values?: MessageValues) => translate(locale, key, values),
    message: (text: string, values?: MessageValues) => translateKnownMessage(locale, text, values),
  }), [locale]);
}

export function LanguageSelect() {
  const { locale, t } = useTranslations();
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [failed, setFailed] = useState(false);
  return (
    <label className={styles.control}>
      <span className="sr-only">{t("Language")}</span>
      <select
        value={locale}
        disabled={pending}
        aria-busy={pending}
        onChange={(event) => {
          const next = event.target.value;
          if (!isLocale(next)) return;
          setFailed(false);
          try {
            document.cookie = languageCookie(next, location.protocol === "https:");
            if (!document.cookie.split(";").some((cookie) => cookie.trim() === `${localeCookie}=${next}`)) {
              setFailed(true);
              return;
            }
          } catch {
            setFailed(true);
            return;
          }
          startTransition(() => router.refresh());
        }}
      >
        {locales.map((value) => <option key={value} value={value} lang={value}>{languageNames[value]}</option>)}
      </select>
      {failed && <span role="status">{t("Language preference could not be saved. Allow cookies and try again.")}</span>}
    </label>
  );
}
