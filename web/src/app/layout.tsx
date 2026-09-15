import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import { getLocale } from "@/lib/i18n/server";
import { LocaleProvider } from "@/lib/i18n/provider";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL || "https://codelocal.cloud"),
  title: {
    default: "CodeLocal",
    template: "%s · CodeLocal",
  },
  description:
    "Connect AI coding clients to an authorized local runtime and durable Project Brain without moving raw source execution into the cloud.",
  manifest: "/manifest.webmanifest",
  applicationName: "CodeLocal",
  appleWebApp: { capable: true, title: "CodeLocal", statusBarStyle: "black-translucent" },
  icons: { icon: "/codelocal-icon.png", apple: "/codelocal-icon.png" },
};

export const viewport = { themeColor: "#07101a", width: "device-width", initialScale: 1, viewportFit: "cover" as const };

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const locale = await getLocale();
  return (
    <html lang={locale} data-scroll-behavior="smooth" className={`${geistSans.variable} ${geistMono.variable}`}>
      <body><LocaleProvider locale={locale}>{children}</LocaleProvider></body>
    </html>
  );
}
