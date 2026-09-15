import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { getScreenshotShareForRender } from "@/lib/screenshot-share-server";
import { getLocale, getTranslations } from "@/lib/i18n/server";
import { LanguageSelect } from "@/lib/i18n/provider";
import { ShareActions } from "./share-actions";
import styles from "./share.module.css";

type SharePageProps = { params: Promise<{ shareID: string }> };

function formatBytes(value: number, locale: string) {
  const megabytes = value >= 1024 * 1024;
  return new Intl.NumberFormat(locale, { style: "unit", unit: megabytes ? "megabyte" : "kilobyte", unitDisplay: "short", maximumFractionDigits: megabytes ? 1 : 0 }).format(megabytes ? value / (1024 * 1024) : Math.max(1, value / 1024));
}

export async function generateMetadata({ params }: SharePageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { shareID } = await params;
  const share = await getScreenshotShareForRender(shareID);
  if (!share) return { title: t("Shot not found"), robots: { index: false, follow: false } };
  const description = t("{width} × {height} screenshot shared with CodeLocal.", { width: share.width, height: share.height });
  const title = `${t("Shared screenshot")} · CodeLocal`;
  return {
    title: t("Shared screenshot"),
    description,
    robots: { index: false, follow: false, nocache: true },
    alternates: { canonical: share.url },
    openGraph: { type: "website", title, description, url: share.url, images: [{ url: share.imageUrl, width: share.width, height: share.height }] },
    twitter: { card: "summary_large_image", title, description, images: [share.imageUrl] },
  };
}

export default async function SharePage({ params }: SharePageProps) {
  const t = await getTranslations();
  const locale = await getLocale();
  const { shareID } = await params;
  const share = await getScreenshotShareForRender(shareID);
  if (!share) notFound();

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <Link className={styles.brand} href="/" aria-label={t("CodeLocal home")}>
          <span><Image src="/codelocal-icon.png" alt="" width={26} height={26} priority /></span>
          <strong>CodeLocal</strong>
        </Link>
        <div className={styles.headerMeta}><span>{t("Shared shot")}</span><code>{share.id}</code></div>
        <div className={styles.headerActions}><LanguageSelect /><Link className={styles.openApp} href="/dashboard">{t("Open app")}</Link></div>
      </header>

      <section className={styles.content}>
        <div className={styles.toolbar}>
          <div>
            <span>{t("Screenshot")}</span>
            <strong>{share.width} × {share.height}</strong>
            <small>{formatBytes(share.size, locale)} · {t("Shared {date}", { date: new Date(share.createdAt).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) })}</small>
          </div>
          <ShareActions shareURL={share.url} downloadURL={share.downloadUrl} />
        </div>

        <figure className={styles.stage}>
          <Image
            alt={t("Screenshot shared with CodeLocal")}
            height={share.height}
            priority
            sizes="(max-width: 1500px) calc(100vw - 48px), 1440px"
            src={share.imageUrl}
            unoptimized
            width={share.width}
          />
        </figure>

        <footer className={styles.footer}>
          <span>{t("Anyone with the link can view")}</span>
          <p>{t("This page is excluded from search engines. The owner can revoke access at any time.")}</p>
          <Link href="/dashboard/shots">{t("Share a screenshot with CodeLocal")} <span aria-hidden="true">↗</span></Link>
        </footer>
      </section>
    </main>
  );
}
