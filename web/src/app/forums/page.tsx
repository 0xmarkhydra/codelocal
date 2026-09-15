import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { AppIcon } from "@/app/dashboard/app-icon";
import { forumExcerpt, getPublicForumTopics, publicForumImageURL } from "@/lib/forum-server";
import type { PublicForumKind, PublicForumStatus } from "@/lib/contracts/forum";
import styles from "./forums.module.css";

export const metadata: Metadata = {
  title: "Forums — Community Support, Bug Reports & Ideas",
  description: "Ask CodeLocal.Cloud questions, report reproducible bugs with screenshots, share workarounds, and follow fixes from community report to GitHub issue and resolution.",
  alternates: { canonical: "/forums" },
  openGraph: {
    type: "website",
    title: "CodeLocal.Cloud Forums",
    description: "Community support, bug reports, screenshots, ideas, and engineering resolutions for CodeLocal.Cloud.",
    url: "/forums",
  },
};

type ForumPageProps = { searchParams: Promise<{ q?: string; kind?: string; status?: string }> };

const kindLabels: Record<PublicForumKind, string> = { question: "Questions", bug: "Bug reports", idea: "Ideas" };
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
  const allTopics = await getPublicForumTopics({limit: 100});
  const hasFilters = Boolean(kind || status || query.q?.trim());
  const topics = hasFilters
    ? await getPublicForumTopics({q: query.q?.trim(), kind, status, limit: 100})
    : allTopics;

  const counts = {
    question: allTopics.filter((topic) => topic.kind === "question").length,
    bug: allTopics.filter((topic) => topic.kind === "bug").length,
    idea: allTopics.filter((topic) => topic.kind === "idea").length,
    resolved: allTopics.filter((topic) => topic.status === "resolved").length,
  };

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "CollectionPage",
    name: "CodeLocal.Cloud Forums",
    description: "Community questions, bug reports and product ideas for CodeLocal.Cloud.",
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

      <section className={styles.communityHero}>
        <div className={styles.container}>
          <div className={styles.heroGrid}>
            <div className={styles.heroCopy}>
              <span className={styles.eyebrow}>05 / Community</span>
              <h1>Build together.<br /><span>Fix faster.</span></h1>
              <p>Questions, bug reports and product ideas from people building with CodeLocal.Cloud. Public by default, useful to everyone.</p>
              <div className={styles.actions}>
                <Link className={styles.primaryButton} href="/dashboard/forums">Start a discussion <AppIcon name="external" size={17} /></Link>
                <a className={styles.secondaryButton} href="#discussions">Browse discussions <AppIcon name="chevron-down" size={17} /></a>
              </div>
            </div>
            <div className={styles.heroSignal} aria-label="Community activity">
              <span>COMMUNITY / LIVE</span>
              <div><strong>{allTopics.length}</strong><small>public discussions</small></div>
              <div><strong>{counts.resolved}</strong><small>resolved threads</small></div>
              <p>Every public report becomes shared context for the next person who hits the same problem.</p>
            </div>
          </div>
        </div>
      </section>

      <section className={styles.pulseBand} aria-label="Community categories">
        <div className={styles.container}>
          <span>Community pulse</span>
          <div><strong>{counts.question} <small>Questions</small></strong><strong>{counts.bug} <small>Bugs</small></strong><strong>{counts.idea} <small>Ideas</small></strong><span>Open knowledge <AppIcon name="connection" size={18} /></span></div>
        </div>
      </section>

      <section className={styles.categoryBand}>
        <div className={styles.container}>
          <div className={styles.sectionHeading}>
            <div><span className={styles.eyebrow}>01 / Find your lane</span><h2>Ask. Report. Shape what comes next.</h2></div>
            <p>Keep the simple things simple. Pick a lane and add only the context that helps others understand it.</p>
          </div>
          <div className={styles.categoryGrid}>
            <Link href="/forums?kind=question" data-active={kind === "question" || undefined}>
              <div><AppIcon name="forum" size={25} /><span>QUESTIONS</span></div>
              <h3>Get unstuck with the community.</h3>
              <p>Ask how something works, share a workaround or help another builder move forward.</p>
              <strong>{counts.question} discussions <AppIcon name="chevron-right" size={17} /></strong>
            </Link>
            <Link href="/forums?kind=bug" data-active={kind === "bug" || undefined}>
              <div><AppIcon name="shield" size={25} /><span>BUG REPORTS</span></div>
              <h3>Show the problem. Follow the fix.</h3>
              <p>Report a bug with a short description and screenshot. Engineering details stay optional.</p>
              <strong>{counts.bug} reports <AppIcon name="chevron-right" size={17} /></strong>
            </Link>
            <Link href="/forums?kind=idea" data-active={kind === "idea" || undefined}>
              <div><AppIcon name="brain" size={25} /><span>IDEAS</span></div>
              <h3>Help shape the product.</h3>
              <p>Share product ideas, workflow improvements and the things you wish CodeLocal.Cloud did next.</p>
              <strong>{counts.idea} ideas <AppIcon name="chevron-right" size={17} /></strong>
            </Link>
          </div>
        </div>
      </section>

      <section className={styles.discussions} id="discussions">
        <div className={styles.container}>
          <div className={styles.sectionHeading}>
            <div><span className={styles.eyebrow}>02 / Discussions</span><h2>What the community is working through.</h2></div>
            <p>Public threads are readable and indexable. Sign in only when you want to post, vote or reply.</p>
          </div>

          <form className={styles.filters} action="/forums" method="get">
            <label className={styles.searchField}><span className={styles.srOnly}>Search forums</span><AppIcon name="search" size={17} /><input name="q" defaultValue={query.q || ""} placeholder="Search discussions" /></label>
            <label><span className={styles.srOnly}>Type</span><select name="kind" defaultValue={kind || ""}><option value="">All types</option><option value="question">Questions</option><option value="bug">Bug reports</option><option value="idea">Ideas</option></select></label>
            <label><span className={styles.srOnly}>Status</span><select name="status" defaultValue={status || ""}><option value="">All statuses</option>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
            <button type="submit">Apply</button>
          </form>

          <div className={styles.listHeader}>
            <div><span>{topics.length}</span> discussions</div>
            {hasFilters && <Link href="/forums">Clear filters</Link>}
          </div>

          {topics.length === 0 ? (
            <section className={styles.empty}><span className={styles.eyebrow}>Nothing here yet</span><h2>No discussions match this filter.</h2><p>Try another search, or start a new topic from the dashboard.</p></section>
          ) : (
            <div className={styles.topicList}>
              {topics.map((topic, index) => (
                <article className={styles.topicRow} key={topic.id}>
                  <span className={styles.topicIndex}>{String(index + 1).padStart(2, "0")}</span>
                  <div className={styles.topicMain}>
                    <div className={styles.badges}><span data-kind={topic.kind}>{kindLabels[topic.kind]}</span><span data-status={topic.status}>{statusLabels[topic.status]}</span>{topic.kind === "bug" && topic.severity && <span>{topic.severity}</span>}</div>
                    <h3><Link href={`/forums/${topic.id}`}>{topic.title}</Link></h3>
                    <p>{forumExcerpt(topic.body, 220)}</p>
                    <footer><time dateTime={new Date(topic.updatedAt).toISOString()}>Updated {forumDate(topic.updatedAt)}</time><span>{topic.voteCount} votes</span><span>{topic.commentCount} replies</span>{topic.githubIssueNumber ? <span>GitHub #{topic.githubIssueNumber}</span> : null}</footer>
                  </div>
                  {topic.assetIds[0] ? (
                    <Link className={styles.topicThumb} href={`/forums/${topic.id}`} aria-label={topic.title}><Image src={publicForumImageURL(topic.assetIds[0], "thumb")} alt="" width={220} height={140} unoptimized /></Link>
                  ) : (
                    <Link className={styles.rowArrow} href={`/forums/${topic.id}`} aria-label={`Open ${topic.title}`}><AppIcon name="chevron-right" size={20} /></Link>
                  )}
                </article>
              ))}
            </div>
          )}
        </div>
      </section>

      <section className={styles.closing}>
        <div className={styles.container}>
          <span className={styles.eyebrow}>Build in public</span>
          <h2>Found something?<br /><span>Make it useful to the next person.</span></h2>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} href="/dashboard/forums">Start a topic <AppIcon name="external" size={17} /></Link>
            <Link className={styles.secondaryButton} href="/support">Talk to us <AppIcon name="chevron-right" size={17} /></Link>
          </div>
        </div>
      </section>
    </main>
  );
}
