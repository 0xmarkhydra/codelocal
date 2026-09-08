import "server-only";
import { cache } from "react";
import { cookies, headers } from "next/headers";
import { localeCookie, resolveLocale } from "./locale";
import { translate, translateKnownMessage, type MessageKey, type MessageValues } from "./messages";

export const getLocale = cache(async () => {
  const preference = (await cookies()).get(localeCookie)?.value;
  return resolveLocale(preference, (await headers()).get("accept-language") ?? "");
});

export async function getTranslations() {
  const locale = await getLocale();
  return Object.assign(
    (key: MessageKey, values?: MessageValues) => translate(locale, key, values),
    { message: (text: string) => translateKnownMessage(locale, text) },
  );
}
