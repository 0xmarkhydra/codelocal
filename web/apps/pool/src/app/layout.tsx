import type { Metadata } from "next";
import { getLocale } from "@codelocal/i18n/server";
import { LocaleProvider } from "@codelocal/i18n/provider";
import "./globals.css";

export const metadata: Metadata = {
  title: "CodeLocal Pool",
  description: "Canonical AI model pool and source router for CodeLocal.",
};

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const locale = await getLocale();
  return (
    <html lang={locale}>
      <body><LocaleProvider locale={locale}>{children}</LocaleProvider></body>
    </html>
  );
}
