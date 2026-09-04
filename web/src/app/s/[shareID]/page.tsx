import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { getScreenshotShareForRender } from "@/lib/screenshot-share-server";
import { ShareActions } from "./share-actions";
import styles from "./share.module.css";

type SharePageProps = { params: Promise<{ shareID: string }> };

function formatBytes(value: number) {
  if (value < 1024 * 1024) return `${Math.max(1, Math.round(value / 1024))} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

export async function generateMetadata({ params }: SharePageProps): Promise<Metadata> {
  const { shareID } = await params;
  const share = await getScreenshotShareForRender(shareID);
  if (!share) return { title: "Shot not found", robots: { index: false, follow: false } };
  const description = `${share.width} × ${share.height} screenshot shared with CodeLocal.`;
  return {
    title: "Shared screenshot",
    description,
    robots: { index: false, follow: false, nocache: true },
    alternates: { canonical: share.url },
    openGraph: { type: "website", title: "Shared screenshot · CodeLocal", description, url: share.url, images: [{ url: share.imageUrl, width: share.width, height: share.height }] },
    twitter: { card: "summary_large_image", title: "Shared screenshot · CodeLocal", description, images: [share.imageUrl] },
  };
}

export default async function SharePage({ params }: SharePageProps) {
  const { shareID } = await params;
  const share = await getScreenshotShareForRender(shareID);
  if (!share) notFound();

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <span><Image src="/codelocal-icon.png" alt="" width={26} height={26} priority /></span>
          <strong>CodeLocal</strong>
        </Link>
        <div className={styles.headerMeta}><i /><span>Shared shot</span><code>{share.id}</code></div>
        <Link className={styles.openApp} href="/dashboard">Open app</Link>
      </header>

      <section className={styles.content}>
        <div className={styles.toolbar}>
          <div>
            <span>Screenshot</span>
            <strong>{share.width} × {share.height}</strong>
            <small>{formatBytes(share.size)} · shared {new Date(share.createdAt).toLocaleString("en", { dateStyle: "medium", timeStyle: "short" })}</small>
          </div>
          <ShareActions shareURL={share.url} downloadURL={share.downloadUrl} />
        </div>

        <figure className={styles.stage}>
          <Image
            alt="Screenshot shared with CodeLocal"
            height={share.height}
            priority
            sizes="(max-width: 1500px) calc(100vw - 48px), 1440px"
            src={share.imageUrl}
            unoptimized
            width={share.width}
          />
        </figure>

        <footer className={styles.footer}>
          <span>Shared privately by link</span>
          <p>This page is excluded from search engines. The owner can revoke access at any time.</p>
          <Link href="/dashboard/shots">Share a screenshot with CodeLocal <span aria-hidden="true">↗</span></Link>
        </footer>
      </section>
    </main>
  );
}
