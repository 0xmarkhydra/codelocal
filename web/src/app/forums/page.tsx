import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { forumExcerpt, getPublicForumTopics, publicForumImageURL } from "@/lib/forum-server";
import type { PublicForumKind, PublicForumStatus } from "@/lib/contracts/forum";
import styles from "./forums.module.css";

export const metadata: Metadata = {
  title: "Forums — Community Support, Bug Reports & Ideas",
  description: "Ask CodeLocal questions, report reproducible bugs with screenshots, share workarounds, and follow fixes from community report to GitHub issue and resolution.",
  alternates: { canonical: "/forums" },
  openGraph: {
    type: "website",
    title: "CodeLocal Forums",
    description: "Community support, bug reports, screenshots, ideas, and engineering resolutions for CodeLocal.",
    url: "/forums",
  },
};

type ForumPageProps = { searchParams: Promise<{ q?: string; kind?: string; status?: string }> };

const kindLabels: Record<PublicForumKind, string> = { question: "Question", bug: "Bug report", idea: "Idea" };
const statusLabels: Record<PublicForumStatus, string> = {
  open: "Open", under_review: "Under review", planned: "Planned", in_progress: "In progress", resolved: "Resolved", closed: "Closed",
};

function forumDate(value: number) {
  return new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(value));
}

export default async function ForumsPage({ searchParams }: ForumPageProps) {
  const query = await searchParams;
  const kind = query.kind === "question" || query.kind === "bug" || query.kind === "idea" ? query.kind : undefined;
  const status = ["open", "under_review", "planned", "in_progress", "resolved", "closed"].includes(query.status || "") ? query.status : undefined;
  const topics = await getPublicForumTopics({ q: query.q?.trim(), kind, status, limit: 100 });

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "CollectionPage",
    name: "CodeLocal Forums",
    description: "Community questions, bug reports and product ideas for CodeLocal.",
    url: "https://codelocal.cloud/forums",
    mainEntity: {
      "@type": "ItemList",
      itemListElement: topics.slice(0, 50).map((topic, index) => ({
        "@type": "ListItem", position: index + 1, url: `https://codelocal.cloud/forums/${topic.id}`, name: topic.title,
      })),
    },
  };

  return (
    <main className={styles.page}>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }} />
      <section className={styles.hero}>
        <div>
          <span className={styles.eyebrow}>CodeLocal Community</span>
          <h1>Forums</h1>
          <p>Ask questions, report bugs with screenshots, share workarounds, and follow engineering fixes in public.</p>
        </div>
        <Link className={styles.primaryButton} href="/dashboard/forums">Start a topic</Link>
      </section>

      <form className={styles.filters} action="/forums" method="get">
        <label className={styles.searchField}><span className={styles.srOnly}>Search forums</span><input name="q" defaultValue={query.q || ""} placeholder="Search questions, bugs and ideas" /></label>
        <label><span className={styles.srOnly}>Type</span><select name="kind" defaultValue={kind || ""}><option value="">All types</option><option value="question">Questions</option><option value="bug">Bug reports</option><option value="idea">Ideas</option></select></label>
        <label><span className={styles.srOnly}>Status</span><select name="status" defaultValue={status || ""}><option value="">All statuses</option>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
        <button type="submit">Search</button>
      </form>

      <section className={styles.listHeader}>
        <div><span>{topics.length}</span> discussions</div>
        <p>Public threads are indexable. Posting and replying requires a CodeLocal account.</p>
      </section>

      {topics.length === 0 ? (
        <section className={styles.empty}><h2>No discussions found</h2><p>Try another filter, or sign in and start the first topic.</p></section>
      ) : (
        <div className={styles.topicList}>
          {topics.map((topic) => (
            <article className={styles.topicCard} key={topic.id}>
              {topic.assetIds[0] && <Link className={styles.topicThumb} href={`/forums/${topic.id}`} aria-label={topic.title}><Image src={publicForumImageURL(topic.assetIds[0], "thumb")} alt="" width={180} height={120} unoptimized /></Link>}
              <div className={styles.topicMain}>
                <div className={styles.badges}><span data-kind={topic.kind}>{kindLabels[topic.kind]}</span><span data-status={topic.status}>{statusLabels[topic.status]}</span>{topic.kind === "bug" && topic.severity && <span>{topic.severity}</span>}</div>
                <h2><Link href={`/forums/${topic.id}`}>{topic.title}</Link></h2>
                <p>{forumExcerpt(topic.body, 220)}</p>
                <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>
                <footer><span>{topic.author}</span><time dateTime={new Date(topic.updatedAt).toISOString()}>Updated {forumDate(topic.updatedAt)}</time><span>{topic.voteCount} votes</span><span>{topic.commentCount} replies</span></footer>
              </div>
            </article>
          ))}
        </div>
      )}
    </main>
  );
}
