import type { Locale } from "@/lib/i18n/locale";
import { translate } from "@/lib/i18n/messages";

export function formatDashboardTime(value: number, locale: Locale = "en") {
  if (!Number.isFinite(value) || value <= 0 || Number.isNaN(new Date(value).getTime())) return translate(locale, "Never");
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
