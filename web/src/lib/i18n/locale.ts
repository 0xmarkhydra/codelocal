export const locales = ["en", "vi", "zh-Hans", "hi"] as const;
export type Locale = (typeof locales)[number];
export const localeCookie = "codelocal-language";
export const languageNames: Record<Locale, string> = {
  en: "English",
  vi: "Tiếng Việt",
  "zh-Hans": "简体中文",
  hi: "हिन्दी",
};

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && locales.some((locale) => locale === value);
}

function matchLanguage(value: string): Locale | undefined {
  try {
    const locale = new Intl.Locale(value);
    if (locale.language === "zh") return "zh-Hans";
    if (locale.language === "en" || locale.language === "vi" || locale.language === "hi") {
      return locale.language;
    }
  } catch {
    // Ignore malformed browser language tags.
  }
}

export function resolveLocale(preference?: string, acceptLanguage = ""): Locale {
  if (isLocale(preference)) return preference;
  const candidates = acceptLanguage.split(",").map((entry, order) => {
    const [tag, ...parameters] = entry.trim().split(";");
    const weight = parameters.find((parameter) => parameter.trim().startsWith("q="));
    const raw = weight?.trim().slice(2);
    const quality = raw === undefined ? 1 : /^(?:0(?:\.\d{0,3})?|1(?:\.0{0,3})?)$/.test(raw) ? Number(raw) : 0;
    return { locale: matchLanguage(tag), quality, order };
  });
  return candidates
    .filter((candidate) => candidate.locale && candidate.quality > 0)
    .sort((a, b) => b.quality - a.quality || a.order - b.order)[0]?.locale ?? "en";
}

export function languageCookie(value: Locale, secure: boolean) {
  if (!isLocale(value)) throw new Error("Unsupported interface language");
  return `${localeCookie}=${value}; Path=/; Max-Age=31536000; SameSite=Lax${secure ? "; Secure" : ""}`;
}
