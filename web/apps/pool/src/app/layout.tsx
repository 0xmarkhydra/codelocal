import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CodeLocal Pool",
  description: "Canonical AI model pool and source router for CodeLocal.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="vi">
      <body>{children}</body>
    </html>
  );
}
